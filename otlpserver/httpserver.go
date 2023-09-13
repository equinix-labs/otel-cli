package otlpserver

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

// HttpServer is a handle for otlp over http/protobuf.
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

// ServeHTTP processes every request as if it is a trace regardless of
// method and path or anything else.
func (hs *HttpServer) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	data, err := io.ReadAll(req.Body)
	if err != nil {
		log.Fatalf("Error while reading request body: %s", err)
	}

	msg := coltracepb.ExportTraceServiceRequest{}
	switch req.Header.Get("Content-Type") {
	case "application/x-protobuf":
		proto.Unmarshal(data, &msg)
	case "application/json":
		json.Unmarshal(data, &msg)
	default:
		rw.WriteHeader(http.StatusNotAcceptable)
	}

	meta := map[string]string{
		"method":       req.Method,
		"proto":        req.Proto,
		"content-type": req.Header.Get("Content-Type"),
		"host":         req.Host,
		"uri":          req.RequestURI,
	}

	headers := make(map[string]string)
	for k := range req.Header {
		headers[k] = req.Header.Get(k)
	}

	done := doCallback(req.Context(), hs.callback, &msg, headers, meta)
	if done {
		go hs.StopWait()
	}
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

// Stop closes the http server and all active connections immediately.
func (hs *HttpServer) Stop() {
	hs.server.Close()
}

// StopWait stops the http server gracefully.
func (hs *HttpServer) StopWait() {
	hs.server.Shutdown(context.Background())
}
