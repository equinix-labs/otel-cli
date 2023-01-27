// otlpserver is a lightweight OTLP/gRPC server implementation intended for use
// in otel-cli and end-to-end testing of OpenTelemetry applications.
package otlpserver

import (
	"context"
	"log"
	"net"
	"strings"
	"sync"

	v1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"

	"google.golang.org/grpc"
)

// Callback is a type for the function passed to newServer that is
// called for each incoming span.
type GrpcCallback func(CliEvent, CliEventList) bool

// GrpcStopper is the function passed to newServer to be called when the
// server is shut down.
type GrpcStopper func(*GrpcServer)

// GrpcServer is a gRPC/OTLP server handle.
type GrpcServer struct {
	server   *grpc.Server
	callback GrpcCallback
	stoponce sync.Once
	stopper  chan struct{}
	stopdone chan struct{}
	doneonce sync.Once
	v1.UnimplementedTraceServiceServer
}

// NewServer takes a callback and stop function and returns a Server ready
// to run with .ServeGRPC().
func NewGrpcServer(cb GrpcCallback, stop GrpcStopper) *GrpcServer {
	s := GrpcServer{
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
		s.server.GracefulStop()
	}()

	return &s
}

// ServeGRPC takes a listener and starts the GRPC server on that listener.
// Blocks until Stop() is called.
func (gs *GrpcServer) ServeGPRC(listener net.Listener) error {
	err := gs.server.Serve(listener)
	gs.stopdone <- struct{}{}
	return err
}

// ListenAndServeGRPC starts a TCP listener then starts the GRPC server using
// ServeGRPC for you.
func (gs *GrpcServer) ListenAndServeGPRC(otlpEndpoint string) {
	otlpEndpoint = strings.TrimPrefix(otlpEndpoint, "grpc://")
	listener, err := net.Listen("tcp", otlpEndpoint)
	if err != nil {
		log.Fatalf("failed to listen on OTLP endpoint %q: %s", otlpEndpoint, err)
	}
	if err := gs.ServeGPRC(listener); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}

// Stop sends a value to the server shutdown goroutine so it stops GRPC
// and calls the stop function given to newServer. Safe to call multiple times.
func (gs *GrpcServer) Stop() {
	gs.stoponce.Do(func() {
		gs.stopper <- struct{}{}
	})
}

// StopWait stops the server and waits for it to affirm shutdown.
func (gs *GrpcServer) StopWait() {
	gs.Stop()
	gs.doneonce.Do(func() {
		<-gs.stopdone
	})
}

// Export implements the gRPC server interface for exporting messages.
func (gs *GrpcServer) Export(ctx context.Context, req *v1.ExportTraceServiceRequest) (*v1.ExportTraceServiceResponse, error) {
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

				f := gs.callback
				done := f(ces, events)
				if done {
					go gs.StopWait()
					return &v1.ExportTraceServiceResponse{}, nil
				}
			}
		}
	}

	return &v1.ExportTraceServiceResponse{}, nil
}
