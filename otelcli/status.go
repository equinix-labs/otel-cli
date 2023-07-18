package otelcli

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	Spans       []map[string]string    `json:"spans"`
	SpanData    map[string]string      `json:"span_data"`
	Env         map[string]string      `json:"env"`
	Diagnostics otlpclient.Diagnostics `json:"diagnostics"`
	Errors      otlpclient.ErrorList   `json:"errors"`
}

func statusCmd(config *otlpclient.Config) *cobra.Command {
	cmd := cobra.Command{
		Use:   "status",
		Short: "send at least one canary and dump status",
		Long: `This subcommand is still experimental and the output format is not yet frozen.

By default just one canary is sent. When --canary-count is set, that number of canaries
are sent. If --canary-interval is set, status will sleep the specified duration
between canaries, up to --timeout (default 1s).

Example:
	otel-cli status
	otel-cli status --canary-count 10 --canary-interval 10 --timeout 10s
`,
		Run: doStatus,
	}

	defaults := otlpclient.DefaultConfig()
	cmd.Flags().IntVar(&config.StatusCanaryCount, "canary-count", defaults.StatusCanaryCount, "number of canaries to send")
	cmd.Flags().StringVar(&config.StatusCanaryInterval, "canary-interval", defaults.StatusCanaryInterval, "number of milliseconds to wait between canaries")

	addCommonParams(&cmd, config)
	addClientParams(&cmd, config)
	addSpanParams(&cmd, config)

	return &cmd
}

func doStatus(cmd *cobra.Command, args []string) {
	var err error
	var exitCode int
	allSpans := []map[string]string{}

	ctx := cmd.Context()
	config := getConfig(ctx)
	ctx, client := StartClient(ctx, config)

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

	var canaryCount int
	var lastSpan *tracepb.Span
	deadline := config.StartupTime.Add(config.ParseCliTimeout())
	interval := config.ParseStatusCanaryInterval()
	for {
		// should be rare but a caller could request 0 canaries, in which case the
		// client will be started and stopped, but no canaries sent
		if config.StatusCanaryCount == 0 {
			// TODO: remove this after SpanData is eliminated
			lastSpan = otlpclient.NewProtobufSpan()
			lastSpan.Name = "unsent canary"
			break
		}

		span := config.NewProtobufSpan()
		span.Name = "otel-cli status"
		if canaryCount > 0 {
			span.Name = fmt.Sprintf("otel-cli status canary %d", canaryCount)
		}
		span.Kind = tracepb.Span_SPAN_KIND_INTERNAL

		// when doing multiple canaries, child each new span to the previous one
		if lastSpan != nil {
			span.TraceId = lastSpan.TraceId
			span.ParentSpanId = lastSpan.SpanId
		}
		lastSpan = span
		allSpans = append(allSpans, otlpclient.SpanToStringMap(span, nil))

		// send it to the server. ignore errors here, they'll happen for sure
		// and the base errors will be tunneled up through otlpclient.GetErrorList()
		ctx, _ = otlpclient.SendSpan(ctx, client, config, span)
		canaryCount++

		if canaryCount == config.StatusCanaryCount {
			break
		} else if time.Now().After(deadline) {
			break
		} else {
			time.Sleep(interval)
		}
	}

	ctx, err = client.Stop(ctx)
	if err != nil {
		config.SoftFail("client.Stop() failed: %s", err)
	}

	// otlpclient saves all errors to a key in context so they can be used
	// to validate assumptions here & in tests
	errorList := otlpclient.GetErrorList(ctx)

	// TODO: does it make sense to turn SpanData into a list of spans?
	outData := StatusOutput{
		Config: config,
		Env:    env,
		Spans:  allSpans,
		// use only the last span's data here, leftover from when status only
		// ever sent one canary
		// legacy, will be removed once test suite is updated
		SpanData: map[string]string{
			"trace_id":   hex.EncodeToString(lastSpan.TraceId),
			"span_id":    hex.EncodeToString(lastSpan.SpanId),
			"is_sampled": strconv.FormatBool(config.GetIsRecording()),
		},
		// Diagnostics is deprecated, being replaced by Errors below and eventually
		// another stringmap of stuff that was tunneled through context.Context
		Diagnostics: otlpclient.Diag,
		Errors:      errorList,
	}

	js, err := json.MarshalIndent(outData, "", "    ")
	config.SoftFailIfErr(err)

	os.Stdout.Write(js)
	os.Stdout.WriteString("\n")

	os.Exit(exitCode)
}
