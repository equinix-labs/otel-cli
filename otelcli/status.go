package otelcli

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
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
// and is also used in ../main_test.go for automated testing.
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
	addSpanParams(statusCmd)
}

func doStatus(cmd *cobra.Command, args []string) {
	exitCode := 0

	// TODO: this always canaries as it is, gotta find the right flags
	// to try to stall sending at the end so as much as possible of the otel
	// code still executes
	span := NewProtobufSpanWithConfig(config)
	span.Name = "otel-cli status"
	span.Kind = tracepb.Span_SPAN_KIND_INTERNAL

	env := make(map[string]string)
	for _, e := range config.envBackup {
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

	// send the span out before printing anything
	err := SendSpan(context.Background(), config, span)
	if err != nil {
		if config.Fail {
			log.Fatalf("%s", err)
		} else {
			softLog("%s", err)
		}
	}
	tp := traceparentFromSpan(span)

	outData := StatusOutput{
		Config: config,
		Env:    env,
		SpanData: map[string]string{
			"trace_id":   hex.EncodeToString(span.TraceId),
			"span_id":    hex.EncodeToString(span.SpanId),
			"is_sampled": strconv.FormatBool(tp.Sampling),
		},
		Diagnostics: diagnostics,
	}

	js, err := json.MarshalIndent(outData, "", "    ")
	if err != nil {
		log.Fatal(err)
	}

	os.Stdout.Write(js)
	os.Stdout.WriteString("\n")

	os.Exit(exitCode)
}
