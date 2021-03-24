package cmd

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var envTp string // global state

// OtelCliCarrier implements the OpenTelemetry TextMapCarrier interface that
// supports only one key/value for the traceparent and does nothing else
type OtelCliCarrier struct{}

func NewOtelCliCarrier() OtelCliCarrier {
	return OtelCliCarrier{}
}

// Get returns the traceparent string if key is "traceparent" otherwise nothing
func (ec OtelCliCarrier) Get(key string) string {
	if key == "traceparent" {
		return envTp
	} else {
		return ""
	}
}

// Set sets the global traceparent if key is "traceparent" otherwise nothing
func (ec OtelCliCarrier) Set(key string, value string) {
	if key == "traceparent" {
		envTp = value
	}
}

// Keys returns a list of strings containing just "traceparent"
func (ec OtelCliCarrier) Keys() []string {
	return []string{"traceparent"}
}

// loadTraceparentFromEnv loads the traceparent from the environment variable
// TRACEPARENT and sets it in the returned Go context.
func loadTraceparentFromEnv(ctx context.Context) context.Context {
	// don't load the envvar when --ignore-tp-env is set
	if ignoreTraceparentEnv {
		return ctx
	}

	tp := os.Getenv("TRACEPARENT")
	if tp == "" {
		return ctx
	}

	// https://github.com/open-telemetry/opentelemetry-go/blob/main/propagation/trace_context.go#L31
	// the 'traceparent' key is a private constant in the otel library so this
	// is using an internal detail but it's probably fine
	carrier := NewOtelCliCarrier()
	carrier.Set("traceparent", tp)
	prop := otel.GetTextMapPropagator()
	return prop.Extract(ctx, carrier)
}

// getTraceparent returns the the traceparent string from the context
// passed in and should reflect the most recent state, e.g. to print out
func getTraceparent(ctx context.Context) string {
	prop := otel.GetTextMapPropagator()
	carrier := NewOtelCliCarrier()
	prop.Inject(ctx, carrier)
	return carrier.Get("traceparent")
}

func printSpanStdout(ctx context.Context, span trace.Span) {
	if !printSpan {
		return
	}

	tpout := getTraceparent(ctx)
	tid := span.SpanContext().TraceID()
	sid := span.SpanContext().SpanID()
	fmt.Printf("# trace id: %s\n#  span id: %s\nTRACEPARENT=%s\n", tid, sid, tpout)
}
