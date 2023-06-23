package otlpclient

import (
	"context"
	"strings"
	"time"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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
func (gc *GrpcClient) Start(ctx context.Context) (context.Context, error) {
	var err error
	endpointURL, _ := ParseEndpoint(gc.config)
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
	isLoopback, err := isLoopbackAddr(endpointURL)
	gc.config.SoftFailIfErr(err)
	if gc.config.Insecure || (isLoopback && !strings.HasPrefix(gc.config.Endpoint, "https")) {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else if !isInsecureSchema(gc.config.Endpoint) {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig(gc.config))))
	}

	// OTLP examples usually show this with the grpc.WithBlock() dial option to
	// make the connection synchronous, but it's not the right default for cli
	// instead, rely on the shutdown methods to make sure everything is flushed
	// by the time the program exits.
	if gc.config.Blocking {
		grpcOpts = append(grpcOpts, grpc.WithBlock())
	}

	ctx, _ = deadlineCtx(ctx, gc.config, gc.config.StartupTime)
	gc.conn, err = grpc.DialContext(ctx, host, grpcOpts...)
	if err != nil {
		gc.config.SoftFail("could not connect to gRPC/OTLP: %s", err)
	}

	gc.client = coltracepb.NewTraceServiceClient(gc.conn)

	return ctx, nil
}

// UploadTraces takes a list of protobuf spans and sends them out, doing retries
// on some errors as needed.
func (gc *GrpcClient) UploadTraces(ctx context.Context, rsps []*tracepb.ResourceSpans) (context.Context, error) {
	// add headers onto the request
	md := metadata.New(gc.config.Headers)
	grpcOpts := []grpc.CallOption{grpc.HeaderCallOption{HeaderAddr: &md}}

	req := coltracepb.ExportTraceServiceRequest{ResourceSpans: rsps}
	ctx, cancel := deadlineCtx(ctx, gc.config, gc.config.StartupTime)
	defer cancel()

	timeout := gc.config.ParseCliTimeout()
	return retry(ctx, gc.config, timeout, func(innerCtx context.Context) (context.Context, bool, time.Duration, error) {
		etsr, err := gc.client.Export(innerCtx, &req, grpcOpts...)
		return processGrpcStatus(innerCtx, etsr, err)
	})
}

// Stop closes the connection to the gRPC server.
func (gc *GrpcClient) Stop(ctx context.Context) (context.Context, error) {
	gc.conn.Close() // ignoring the error
	return ctx, nil
}

func processGrpcStatus(ctx context.Context, etsr *coltracepb.ExportTraceServiceResponse, err error) (context.Context, bool, time.Duration, error) {
	if err == nil {
		// success!
		return ctx, false, 0, nil
	}

	st := status.Convert(err)
	if st.Code() == codes.OK {
		// apparently this can happen and is a success
		return ctx, false, 0, nil
	}

	var ri *errdetails.RetryInfo
	for _, d := range st.Details() {
		if t, ok := d.(*errdetails.RetryInfo); ok {
			ri = t
		}
	}

	// handle retriable codes, somewhat lifted from otel collector
	switch st.Code() {
	case codes.Aborted,
		codes.Canceled,
		codes.DataLoss,
		codes.DeadlineExceeded,
		codes.OutOfRange,
		codes.Unavailable:
		return ctx, true, 0, err
	case codes.ResourceExhausted:
		// only retry this one if RetryInfo was set
		if ri != nil && ri.RetryDelay != nil {
			// when RetryDelay is available, pass it back to the retry loop
			// so it can sleep that duration
			wait := time.Duration(ri.RetryDelay.Seconds)*time.Second + time.Duration(ri.RetryDelay.Nanos)*time.Nanosecond
			return ctx, true, wait, err
		} else {
			return ctx, false, 0, err
		}
	default:
		// don't retry anything else
		return ctx, false, 0, err
	}

}
