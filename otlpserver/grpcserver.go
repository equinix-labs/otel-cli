package otlpserver

import (
	"bytes"
	"context"
	"encoding/csv"
	"log"
	"net"
	"sync"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// GrpcServer is a gRPC/OTLP server handle.
type GrpcServer struct {
	server   *grpc.Server
	callback Callback
	stoponce sync.Once
	stopper  chan struct{}
	stopdone chan struct{}
	doneonce sync.Once
	coltracepb.UnimplementedTraceServiceServer
}

// NewGrpcServer takes a callback and stop function and returns a Server ready
// to run with .Serve().
func NewGrpcServer(cb Callback, stop Stopper) *GrpcServer {
	s := GrpcServer{
		server:   grpc.NewServer(),
		callback: cb,
		stopper:  make(chan struct{}),
		stopdone: make(chan struct{}, 1),
	}

	coltracepb.RegisterTraceServiceServer(s.server, &s)

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
func (gs *GrpcServer) Serve(listener net.Listener) error {
	err := gs.server.Serve(listener)
	gs.stopdone <- struct{}{}
	return err
}

// ListenAndServeGRPC starts a TCP listener then starts the GRPC server using
// ServeGRPC for you.
func (gs *GrpcServer) ListenAndServe(otlpEndpoint string) {
	listener, err := net.Listen("tcp", otlpEndpoint)
	if err != nil {
		log.Fatalf("failed to listen on OTLP endpoint %q: %s", otlpEndpoint, err)
	}
	if err := gs.Serve(listener); err != nil {
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
func (gs *GrpcServer) Export(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
	// OTLP/gRPC headers are passed in metadata, copy them to serverMeta
	// for now. This isn't ideal but gets them exposed to the test suite.
	headers := make(map[string]string)
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		for mdk := range md {
			vals := md.Get(mdk)
			buf := bytes.NewBuffer([]byte{})
			csv.NewWriter(buf).WriteAll([][]string{vals})
			headers[mdk] = buf.String()
		}
	}

	done := doCallback(ctx, gs.callback, req, headers, map[string]string{"proto": "grpc"})
	if done {
		go gs.StopWait()
	}
	return &coltracepb.ExportTraceServiceResponse{}, nil
}
