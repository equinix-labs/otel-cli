// otlpserver is an OTLP server with HTTP and gRPC backends available.
// It takes a lot of shortcuts to keep things simple and is not intended
// to be used as a serious OTLP service. Primarily it is for the test
// suite and also supports the otel-cli server features.
package otlpserver

import (
	"net"
)

// Callback is a type for the function passed to newServer that is
// called for each incoming span.
type Callback func(CliEvent, CliEventList) bool

// GrpcStopper is the function passed to newServer to be called when the
// server is shut down.
type Stopper func(OtlpServer)

// OtlpServer abstracts the minimum interface required for an OTLP
// server to be either HTTP or gRPC (but not both, for now).
type OtlpServer interface {
	ListenAndServe(otlpEndpoint string)
	Serve(listener net.Listener) error
	Stop()
	StopWait()
}

// NewServer will start the requested server protocol, one of grpc, http/protobuf,
// and http/json.
func NewServer(protocol string, cb Callback, stop Stopper) OtlpServer {
	switch protocol {
	case "grpc":
		return NewGrpcServer(cb, stop)
	case "http":
		return NewHttpServer(cb, stop)
	}

	return nil
}
