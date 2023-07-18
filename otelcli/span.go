package otelcli

import (
	"os"

	"github.com/equinix-labs/otel-cli/otlpclient"
	"github.com/spf13/cobra"
)

// spanCmd represents the span command
func spanCmd(config *Config) *cobra.Command {
	cmd := cobra.Command{
		Use:   "span",
		Short: "create an OpenTelemetry span and send it",
		Long: `Create an OpenTelemetry span as specified and send it along. The
span can be customized with a start/end time via RFC3339 or Unix epoch format,
with support for nanoseconds on both.

Example:
	otel-cli span \
		--service "my-application" \
		--name "send data to the server" \
		--start 2021-03-24T07:28:05.12345Z \
		--end $(date +%s.%N) \
		--attrs "os.kernel=$(uname -r)" \
		--tp-print
`,
		Run: doSpan,
	}

	cmd.Flags().SortFlags = false

	addCommonParams(&cmd, config)
	addSpanParams(&cmd, config)
	addSpanStartEndParams(&cmd, config)
	addAttrParams(&cmd, config)
	addClientParams(&cmd, config)

	// subcommands
	cmd.AddCommand(spanBgCmd(config))
	cmd.AddCommand(spanEventCmd(config))
	cmd.AddCommand(spanEndCmd(config))

	return &cmd
}

func doSpan(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()
	config := getConfig(ctx)
	ctx, client := StartClient(ctx, config)
	span := config.NewProtobufSpan()
	ctx, err := otlpclient.SendSpan(ctx, client, config, span)
	config.SoftFailIfErr(err)
	_, err = client.Stop(ctx)
	config.SoftFailIfErr(err)
	config.PropagateTraceparent(span, os.Stdout)
}
