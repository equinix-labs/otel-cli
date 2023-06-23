package otlpclient

import (
	"context"

	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// NullClient is an OTLP client backend for non-recording mode that drops
// all data and never returns errors.
type NullClient struct{}

// NewNullClient returns a fresh NullClient ready to Start.
func NewNullClient(config Config) *NullClient {
	return &NullClient{}
}

// Start fulfills the interface and does nothing.
func (nc *NullClient) Start(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

// UploadTraces fulfills the interface and does nothing.
func (nc *NullClient) UploadTraces(ctx context.Context, rsps []*tracepb.ResourceSpans) (context.Context, error) {
	return ctx, nil
}

// Stop fulfills the interface and does nothing.
func (gc *NullClient) Stop(ctx context.Context) (context.Context, error) {
	return ctx, nil
}
