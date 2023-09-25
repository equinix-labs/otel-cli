package otlpclient

import (
	"context"
	"fmt"
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
	config OTLPConfig
}

// NewGrpcClient returns a fresh GrpcClient ready to Start.
func NewGrpcClient(config OTLPConfig) *GrpcClient {
	c := GrpcClient{config: config}
	return &c
}

// Start configures and starts the connection to the gRPC server in the background.
func (gc *GrpcClient) Start(ctx context.Context) (context.Context, error) {
	var err error
	endpointURL := gc.config.GetEndpoint()
	host := endpointURL.Hostname()
	if endpointURL.Port() != "" {
		host = host + ":" + endpointURL.Port()
	}

	grpcOpts := []grpc.DialOption{}

	if gc.config.GetInsecure() {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(credentials.NewTLS(gc.config.GetTlsConfig())))
	}

	gc.conn, err = grpc.DialContext(ctx, host, grpcOpts...)
	if err != nil {
		return ctx, fmt.Errorf("could not connect to gRPC/OTLP: %w", err)
	}

	gc.client = coltracepb.NewTraceServiceClient(gc.conn)

	return ctx, nil
}

// UploadTraces takes a list of protobuf spans and sends them out, doing retries
// on some errors as needed.
// TODO: look into grpc.WaitForReady(), esp for status use cases
func (gc *GrpcClient) UploadTraces(ctx context.Context, rsps []*tracepb.ResourceSpans) (context.Context, error) {
	// add headers onto the request
	headers := gc.config.GetHeaders()
	if len(headers) > 0 {
		md := metadata.New(headers)
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	req := coltracepb.ExportTraceServiceRequest{ResourceSpans: rsps}

	return retry(ctx, gc.config, func(innerCtx context.Context) (context.Context, bool, time.Duration, error) {
		etsr, err := gc.client.Export(innerCtx, &req)
		return processGrpcStatus(innerCtx, etsr, err)
	})
}

// Stop closes the connection to the gRPC server.
func (gc *GrpcClient) Stop(ctx context.Context) (context.Context, error) {
	return ctx, gc.conn.Close()
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
