package otelcmd

import (
	"os"
	"regexp"

	"github.com/equinix-labs/otel-cli/otelcli"
	"github.com/spf13/cobra"
)

var epochNanoTimeRE *regexp.Regexp

func init() {
	epochNanoTimeRE = regexp.MustCompile(`^\d+\.\d+$`)
}

// spanCmd represents the span command
func spanCmd(config *otelcli.Config) *cobra.Command {
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
	ctx, client := otelcli.StartClient(ctx, config)
	span := otelcli.NewProtobufSpanWithConfig(config)
	otelcli.SendSpan(ctx, client, config, span)
	otelcli.PropagateTraceparent(config, span, os.Stdout)
}
