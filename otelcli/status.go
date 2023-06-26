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
	allSpans := []map[string]string{}
	ctx := cmd.Context()
	config := getConfig(ctx)
	ctx, client := otlpclient.StartClient(ctx, config)

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

	// subtract one keepalive from the timeout so the canarying loop will stop
	// before the deadline and subsequent timeouts
	// TODO: handle cases where keepalive > timeout, but not worried since this tool
	// is mostly for testing use cases
	keepalive := time.Millisecond * time.Duration(config.StatusKeepaliveMs)
	deadline := config.StartupTime.Add(config.ParseCliTimeout() - keepalive)
	var canaryCount int
	var lastSpan *tracepb.Span
	for {
		span := otlpclient.NewProtobufSpanWithConfig(config)
		span.Name = "otel-cli status"
		if canaryCount > 0 {
			span.Name = fmt.Sprintf("otel-cli status canary %d", canaryCount)
		}
		span.Kind = tracepb.Span_SPAN_KIND_INTERNAL

		// when doing --keepalive, child each new span to the previous one
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

		if config.StatusKeepaliveMs == 0 || time.Now().After(deadline) {
			break
		} else {
			time.Sleep(keepalive)
		}
	}

	ctx, err = client.Stop(ctx)
	if err != nil {
		config.SoftFail("client.Stop() failed: %s", err)
	}

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
			"is_sampled": strconv.FormatBool(config.IsRecording()),
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
