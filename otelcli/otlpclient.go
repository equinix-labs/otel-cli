package otelcli

import (
	"context"
	"fmt"
	"strings"

	"github.com/equinix-labs/otel-cli/otlpclient"
)

// StartClient uses the Config to setup and start either a gRPC or HTTP client,
// and returns the OTLPClient interface to them.
func StartClient(ctx context.Context, config otlpclient.Config) (context.Context, otlpclient.OTLPClient) {
	if !config.GetIsRecording() {
		return ctx, otlpclient.NewNullClient(config)
	}

	if config.Protocol != "" && config.Protocol != "grpc" && config.Protocol != "http/protobuf" {
		err := fmt.Errorf("invalid protocol setting %q", config.Protocol)
		otlpclient.Diag.Error = err.Error()
		config.SoftFail(err.Error())
	}

	endpointURL := config.GetEndpoint()

	var client otlpclient.OTLPClient
	if config.Protocol != "grpc" &&
		(strings.HasPrefix(config.Protocol, "http/") ||
			endpointURL.Scheme == "http" ||
			endpointURL.Scheme == "https") {
		client = otlpclient.NewHttpClient(config)
	} else {
		client = otlpclient.NewGrpcClient(config)
	}

	ctx, err := client.Start(ctx)
	if err != nil {
		otlpclient.Diag.Error = err.Error()
		config.SoftFail("Failed to start OTLP client: %s", err)
	}

	return ctx, client
}
