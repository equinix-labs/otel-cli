package otlpserver

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"

	v1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

const (
	pbContentType   = "application/x-protobuf"
	jsonContentType = "application/json"
)

// see: https://github.com/open-telemetry/opentelemetry-collector/blob/e5208293ec5d4e04939ff52d60519ddbaa12d87a/pdata/internal/data/protogen/collector/trace/v1/trace_service.pb.go#L33
type ExportTraceServiceRequest struct {
	ResourceSpans []*v1.ResourceSpans `protobuf:"bytes,1,rep,name=resource_spans,json=resourceSpans,proto3" json:"resource_spans,omitempty"`
}

type HttpServer struct {
	server   *http.Server
	callback Callback
}

// NewServer takes a callback and stop function and returns a Server ready
// to run with .Serve().
func NewHttpServer(cb Callback, stop Stopper) *HttpServer {
	s := HttpServer{
		server:   &http.Server{},
		callback: cb,
	}

	s.server.Handler = &s

	return &s
}

func (hs *HttpServer) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	data, err := io.ReadAll(req.Body)
	if err != nil {
		log.Fatalf("Error while reading request body: %s", err)
	}

	fmt.Fprintf(os.Stderr, "\n\n%q\n\n", data)
}

// ServeHttp takes a listener and starts the HTTP server on that listener.
// Blocks until Stop() is called.
func (hs *HttpServer) Serve(listener net.Listener) error {
	err := hs.server.Serve(listener)
	return err
}

// ListenAndServeHttp starts a TCP listener then starts the HTTP server using
// ServeHttp for you.
func (hs *HttpServer) ListenAndServe(otlpEndpoint string) {
	listener, err := net.Listen("tcp", otlpEndpoint)
	if err != nil {
		log.Fatalf("failed to listen on OTLP endpoint %q: %s", otlpEndpoint, err)
	}
	if err := hs.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}

// Stop sends a value to the server shutdown goroutine so it stops HTTP
// and calls the stop function given to newServer. Safe to call multiple times.
func (hs *HttpServer) Stop() {
}

// StopWait stops the server and waits for it to affirm shutdown.
func (hs *HttpServer) StopWait() {
}
