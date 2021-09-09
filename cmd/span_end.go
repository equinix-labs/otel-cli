package cmd

import (
	"log"
	"os"

	"github.com/spf13/cobra"
)

// spanEndCmd represents the span event command
var spanEndCmd = &cobra.Command{
	Use:   "end",
	Short: "Make a span background to end itself and exit gracefully",
	Long: `Gracefully end a background span and have its process exit.

See: otel-cli span background

	otel-cli span end --sockdir $sockdir
`,
	Run: doSpanEnd,
}

func init() {
	spanCmd.AddCommand(spanEndCmd)
	spanEndCmd.Flags().StringVar(&config.BackgroundSockdir, "sockdir", "", "a directory where a socket can be placed safely")
	spanEndCmd.MarkFlagRequired("sockdir")
}

func doSpanEnd(cmd *cobra.Command, args []string) {
	client, shutdown := createBgClient()

	rpcArgs := BgEnd{}
	res := BgSpan{}
	err := client.Call("BgSpan.End", rpcArgs, &res)
	if err != nil {
		log.Fatalf("error while calling background server rpc BgSpan.End: %s", err)
	}
	shutdown()

	printSpanData(os.Stdout, res.TraceID, res.SpanID, res.Traceparent)
}
