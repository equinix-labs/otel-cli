package otelcli

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/equinix-labs/otel-cli/otlpclient"
	"github.com/spf13/cobra"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// StatusOutput captures all the data we want to print out for this subcommand
// and is also used in ../main_test.go for automated testing.
type StatusOutput struct {
	Config      otlpclient.Config      `json:"config"`
	SpanData    map[string]string      `json:"span_data"`
	Env         map[string]string      `json:"env"`
	Diagnostics otlpclient.Diagnostics `json:"diagnostics"`
	Errors      otlpclient.ErrorList   `json:"errors"`
}

func statusCmd(config *otlpclient.Config) *cobra.Command {
	cmd := cobra.Command{
		Use:   "status",
		Short: "start up otel and dump status, optionally sending a canary span",
		Long: `This subcommand is still experimental and the output format is not yet frozen.

By default just one canary span is sent. When --keepalive is set to some number of milliseconds,
otel-cli status will try to send a span each n ms until the --timeout value is reached.

Example:
	otel-cli status
	otel-cli status --keepalive 1000 --timeout 10
`,
		Run: doStatus,
	}

	defaults := otlpclient.DefaultConfig()
	cmd.Flags().IntVar(&config.StatusKeepaliveMs, "keepalive", defaults.StatusKeepaliveMs, "number of milliseconds to wait between keepalive spans")

	addCommonParams(&cmd, config)
	addClientParams(&cmd, config)
	addSpanParams(&cmd, config)

	return &cmd
}

func doStatus(cmd *cobra.Command, args []string) {
	var err error
	var exitCode int
	ctx := cmd.Context()
	config := getConfig(ctx)
	ctx, client := otlpclient.StartClient(ctx, config)

	// TODO: this always canaries as it is, gotta find the right flags
	// to try to stall sending at the end so as much as possible of the otel
	// code still executes
	span := otlpclient.NewProtobufSpanWithConfig(config)
	span.Name = "otel-cli status"
	span.Kind = tracepb.Span_SPAN_KIND_INTERNAL

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
			config.SoftFail("BUG in otel-cli: this shouldn't happen")
		}
	}

	deadline := config.StartupTime.Add(config.ParseCliTimeout())
	var canaryCount int
	for {
		// send the span out before printing anything
		log.Printf("canary %d\n", canaryCount)
		ctx, err = otlpclient.SendSpan(ctx, client, config, span)
		if err != nil {
			if config.Fail {
				log.Fatalf("%s", err)
			} else {
				config.SoftLog("%s", err)
			}
		}
		canaryCount++

		if config.StatusKeepaliveMs == 0 || time.Now().After(deadline) {
			log.Printf("breaking now...\n")
			break
		} else {
			log.Printf("sleeping now...\n")
			time.Sleep(time.Millisecond * time.Duration(config.StatusKeepaliveMs))
		}
	}
	log.Printf("cleaning up now...\n")

	ctx, err = client.Stop(ctx)
	if err != nil {
		config.SoftFail("client.Stop() failed: %s", err)
	}

	errorList := otlpclient.GetErrorList(ctx)

	outData := StatusOutput{
		Config: config,
		Env:    env,
		SpanData: map[string]string{
			"trace_id":   hex.EncodeToString(span.TraceId),
			"span_id":    hex.EncodeToString(span.SpanId),
			"is_sampled": strconv.FormatBool(config.IsRecording()),
		},
		Diagnostics: otlpclient.Diag,
		Errors:      errorList,
	}

	js, err := json.MarshalIndent(outData, "", "    ")
	if err != nil {
		log.Fatal(err)
	}

	os.Stdout.Write(js)
	os.Stdout.WriteString("\n")

	os.Exit(exitCode)
}
