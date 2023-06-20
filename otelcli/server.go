package otelcli

import (
	"strings"

	"github.com/equinix-labs/otel-cli/otlpclient"
	"github.com/equinix-labs/otel-cli/otlpserver"
	"github.com/spf13/cobra"
)

const defaultOtlpEndpoint = "grpc://localhost:4317"
const spanBgSockfilename = "otel-cli-background.sock"

func serverCmd(config *otlpclient.Config) *cobra.Command {
	cmd := cobra.Command{
		Use:   "server",
		Short: "run an embedded OTLP server",
		Long:  "Run otel-cli as an OTLP server. See subcommands.",
	}

	cmd.AddCommand(serverJsonCmd(config))
	cmd.AddCommand(serverTuiCmd(config))

	return &cmd
}

// runServer runs the server on either grpc or http and blocks until the server
// stops or is killed.
func runServer(config otlpclient.Config, cb otlpserver.Callback, stop otlpserver.Stopper) {
	// unlike the rest of otel-cli, server should default to localhost:4317
	if config.Endpoint == "" {
		config.Endpoint = defaultOtlpEndpoint
	}
	endpointURL, _ := otlpclient.ParseEndpoint(config)

	var cs otlpserver.OtlpServer
	if config.Protocol != "grpc" &&
		(strings.HasPrefix(config.Protocol, "http/") ||
			endpointURL.Scheme == "http") {
		cs = otlpserver.NewServer("http", cb, stop)
	} else if config.Protocol == "https" || endpointURL.Scheme == "https" {
		config.SoftFail("https server is not supported yet, please raise an issue")
	} else {
		cs = otlpserver.NewServer("grpc", cb, stop)
	}

	defer cs.Stop()
	cs.ListenAndServe(endpointURL.Host)
}
