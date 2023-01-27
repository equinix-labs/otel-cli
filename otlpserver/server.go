package otlpserver

import (
	"net"
)

// Callback is a type for the function passed to newServer that is
// called for each incoming span.
type Callback func(CliEvent, CliEventList) bool

// GrpcStopper is the function passed to newServer to be called when the
// server is shut down.
type Stopper func(*GrpcServer)

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
	case "http/protobuf":
	case "http/json":
	}

	return nil
}
