// otlpserver is a lightweight OTLP/gRPC server implementation intended for use
// in otel-cli and end-to-end testing of OpenTelemetry applications.
package otlpserver

import (
	"context"
	"log"
	"net"
	"sync"

	v1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"

	"google.golang.org/grpc"
)

// Callback is a type for the function passed to newServer that is
// called for each incoming span.
type Callback func(CliEvent, CliEventList) bool

// Stopper is the function passed to newServer to be called when the
// server is shut down.
type Stopper func(*Server)

// Server is a gRPC/OTLP server handle.
type Server struct {
	server   *grpc.Server
	callback Callback
	stoponce sync.Once
	stopper  chan struct{}
	stopdone chan struct{}
	doneonce sync.Once
	v1.UnimplementedTraceServiceServer
}

// NewServer takes a callback and stop function and returns a Server ready
// to run with .ServeGRPC().
func NewServer(cb Callback, stop Stopper) *Server {
	s := Server{
		server:   grpc.NewServer(),
		callback: cb,
		stopper:  make(chan struct{}),
		stopdone: make(chan struct{}, 1),
	}

	v1.RegisterTraceServiceServer(s.server, &s)

	// single place to stop the server, used by timeout and max-spans
	go func() {
		<-s.stopper
		stop(&s)
		s.server.Stop()
	}()

	return &s
}

// ServeGRPC takes a listener and starts the GRPC server on that listener.
// Blocks until Stop() is called.
func (cs *Server) ServeGPRC(listener net.Listener) error {
	err := cs.server.Serve(listener)
	cs.stopdone <- struct{}{}
	return err
}

// ListenAndServeGRPC starts a TCP listener then starts the GRPC server using
// ServeGRPC for you.
func (cs *Server) ListenAndServeGPRC(otlpEndpoint string) {
	listener, err := net.Listen("tcp", otlpEndpoint)
	if err != nil {
		log.Fatalf("failed to listen on OTLP endpoint %q: %s", otlpEndpoint, err)
	}
	if err := cs.ServeGPRC(listener); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}

// Stop sends a value to the server shutdown goroutine so it stops GRPC
// and calls the stop function given to newServer. Safe to call multiple times.
func (cs *Server) Stop() {
	cs.stoponce.Do(func() {
		cs.stopper <- struct{}{}
	})
}

// StopWait stops the server and waits for it to affirm shutdown.
func (cs *Server) StopWait() {
	cs.Stop()
	cs.doneonce.Do(func() {
		<-cs.stopdone
	})
}

// Export implements the gRPC server interface for exporting messages.
func (cs *Server) Export(ctx context.Context, req *v1.ExportTraceServiceRequest) (*v1.ExportTraceServiceResponse, error) {
	rss := req.GetResourceSpans()
	for _, resource := range rss {
		scopeSpans := resource.GetScopeSpans()
		for _, ss := range scopeSpans {
			for _, span := range ss.GetSpans() {
				// convert protobuf spans to something easier for humans to consume
				ces := NewCliEventFromSpan(span, ss, resource)
				events := CliEventList{}
				for _, se := range span.GetEvents() {
					events = append(events, NewCliEventFromSpanEvent(se, span, ss))
				}

				f := cs.callback
				done := f(ces, events)
				if done {
					return &v1.ExportTraceServiceResponse{}, nil
				}
			}
		}
	}

	return &v1.ExportTraceServiceResponse{}, nil
}
