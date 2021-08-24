// otlpserver is a lightweight OTLP/gRPC server implementation intended for use
// in otel-cli and end-to-end testing of OpenTelemetry applications.
package otlpserver

import (
	"context"
	"log"
	"net"

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
	stopper  chan bool
	v1.UnimplementedTraceServiceServer
}

// NewServer takes a callback and stop function and returns a Server ready
// to run with .ServeGRPC().
func NewServer(cb Callback, stop Stopper) *Server {
	s := Server{
		server:   grpc.NewServer(),
		callback: cb,
		stopper:  make(chan bool),
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

// ServeGRPC creates a listener on otlpEndpoint and starts the GRPC server
// on that listener. Blocks until stopped by sending a value to cs.stopper.
func (cs *Server) ServeGPRC(endpoint string) {
	listener, err := net.Listen("tcp", endpoint)
	if err != nil {
		log.Fatalf("failed to listen on %q: %s", endpoint, err)
	}

	if err := cs.server.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}

// Stop sends a value to the server shutdown goroutine so it stops GRPC
// and calls the stop function given to newServer.
func (cs *Server) Stop() {
	cs.stopper <- true
}

// Export implements the gRPC server interface for exporting messages.
func (cs *Server) Export(ctx context.Context, req *v1.ExportTraceServiceRequest) (*v1.ExportTraceServiceResponse, error) {
	rss := req.GetResourceSpans()
	for _, resource := range rss {
		ilSpans := resource.GetInstrumentationLibrarySpans()
		for _, ils := range ilSpans {
			for _, span := range ils.GetSpans() {
				// convert protobuf spans to something easier for humans to consume
				ces := NewCliEventFromSpan(span, ils)
				events := CliEventList{}
				for _, se := range span.GetEvents() {
					events = append(events, NewCliEventFromSpanEvent(se, span, ils))
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
