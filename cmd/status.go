package cmd

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "start up otel and dump status, optionally sending a canary span",
	Long: `This subcommand is still experimental and the output format is not yet frozen.
Example:
	otel-cli status
`,
	Run: doStatus,
}

// StatusOutput captures all the data we want to print out for this subcommand
// and is also is used in ../main_test.go for automated testing.
type StatusOutput struct {
	Config      Config            `json:"config"`
	SpanData    map[string]string `json:"span_data"`
	Env         map[string]string `json:"env"`
	Diagnostics Diagnostics       `json:"diagnostics"`
}

func init() {
	rootCmd.AddCommand(statusCmd)
	addCommonParams(statusCmd)
	addClientParams(statusCmd)
}

func doStatus(cmd *cobra.Command, args []string) {
	ctx, shutdown := initTracer()

	// TODO: this always canaries as it is, gotta find the right flags
	// to try to stall sending at the end so as much as possible of the otel
	// code still executes
	tracer := otel.Tracer("otel-cli/status")
	ctx, span := tracer.Start(ctx, "dump state")
	diagnostics.IsRecording = span.IsRecording()

	env := make(map[string]string)
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			// TODO: this is just enough so I can sleep tonight.
			// should be a list at top of file and needs a flag to turn it off
			// TODO: for sure need to mask OTEL_EXPORTER_OTLP_HEADERS
			if strings.Contains(strings.ToLower(parts[0]), "token") || parts[0] == "OTEL_EXPORTER_OTLP_HEADERS" {
				env[parts[0]] = "--- redacted ---"
			} else {
				env[parts[0]] = parts[1]
			}
		} else {
			softFail("BUG in otel-cli: this shouldn't happen")
		}
	}

	// grab the SpanContext for printing things, end the span, then shut down
	// otel before moving on to print things
	sc := trace.SpanContextFromContext(ctx)
	span.End()
	shutdown()

	outData := StatusOutput{
		Config: config,
		Env:    env,
		SpanData: map[string]string{
			"trace_id":    sc.TraceID().String(),
			"span_id":     sc.SpanID().String(),
			"trace_flags": sc.TraceFlags().String(),
			"is_sampled":  strconv.FormatBool(sc.IsSampled()),
		},
		Diagnostics: diagnostics,
	}

	js, err := json.MarshalIndent(outData, "", "    ")
	if err != nil {
		log.Fatal(err)
	}

	os.Stdout.Write(js)
	os.Stdout.WriteString("\n")
}
