package otelcli

import (
	"context"
	"strings"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// GrpcClient holds the state for gRPC connections.
type GrpcClient struct {
	conn   *grpc.ClientConn
	client coltracepb.TraceServiceClient
	config Config
}

// NewGrpcClient returns a fresh GrpcClient ready to Start.
func NewGrpcClient(config Config) *GrpcClient {
	c := GrpcClient{config: config}
	return &c
}

// Start configures and starts the connection to the gRPC server in the background.
func (gc *GrpcClient) Start(ctx context.Context) error {
	endpointURL, _ := parseEndpoint(config)
	host := endpointURL.Hostname()
	if endpointURL.Port() != "" {
		host = host + ":" + endpointURL.Port()
	}

	grpcOpts := []grpc.DialOption{}

	// Go's TLS does the right thing and forces us to say we want to disable encryption,
	// but I expect most users of this program to point at a localhost endpoint that might not
	// have any encryption available, or setting it up raises the bar of entry too high.
	// The compromise is to automatically flip this flag to true when endpoint contains an
	// an obvious "localhost", "127.0.0.x", or "::1" address.
	if config.Insecure || (isLoopbackAddr(endpointURL) && !strings.HasPrefix(config.Endpoint, "https")) {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else if !isInsecureSchema(config.Endpoint) {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig(config))))
	}

	// OTLP examples usually show this with the grpc.WithBlock() dial option to
	// make the connection synchronous, but it's not the right default for cli
	// instead, rely on the shutdown methods to make sure everything is flushed
	// by the time the program exits.
	if config.Blocking {
		grpcOpts = append(grpcOpts, grpc.WithBlock())
	}

	var err error
	ctx, _ = deadlineCtx(ctx, gc.config, gc.config.startupTime)
	gc.conn, err = grpc.DialContext(ctx, host, grpcOpts...)
	if err != nil {
		softFail("could not connect to gRPC/OTLP: %s", err)
	}

	gc.client = coltracepb.NewTraceServiceClient(gc.conn)

	return nil
}

// UploadTraces takes a list of protobuf spans and sends them out, doing retries
// on some errors as needed.
func (gc *GrpcClient) UploadTraces(ctx context.Context, rsps []*tracepb.ResourceSpans) error {
	// add headers onto the request
	md := metadata.New(config.Headers)
	grpcOpts := []grpc.CallOption{grpc.HeaderCallOption{HeaderAddr: &md}}

	req := coltracepb.ExportTraceServiceRequest{ResourceSpans: rsps}
	ctx, cancel := deadlineCtx(ctx, gc.config, gc.config.startupTime)
	defer cancel()

	timeout := parseCliTimeout(config)
	return retry(timeout, func() (bool, error) {
		etsr, err := gc.client.Export(ctx, &req, grpcOpts...)
		return processGrpcStatus(etsr, err)
	})
}

// Stop closes the connection to the gRPC server.
func (gc *GrpcClient) Stop(ctx context.Context) error {
	return gc.conn.Close()
}

// func processHTTPStatus(resp *http.Response, body []byte) (bool, error)
func processGrpcStatus(etsr *coltracepb.ExportTraceServiceResponse, err error) (bool, error) {
	if err != nil {
		return true, err
	} else {
		return false, nil
	}
}
