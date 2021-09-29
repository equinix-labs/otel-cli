package cmd

import (
	"bytes"
	"context"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"

	"go.opentelemetry.io/otel"
)

var envTp string // global state
var checkTracecarrierRe *regexp.Regexp

// OtelCliCarrier implements the OpenTelemetry TextMapCarrier interface that
// supports only one key/value for the traceparent and does nothing else
type OtelCliCarrier struct{}

func init() {
	// only anchored at the front because traceparents can include more things
	// per the standard but only the first 4 are required for our uses
	checkTracecarrierRe = regexp.MustCompile("^[[:xdigit:]]{2}-[[:xdigit:]]{32}-[[:xdigit:]]{16}-[[:xdigit:]]{2}")
}

// NewOtelCliCarrier returns a default otel carrier struct
func NewOtelCliCarrier() OtelCliCarrier {
	return OtelCliCarrier{}
}

// Get returns the traceparent string if key is "traceparent" otherwise nothing
func (ec OtelCliCarrier) Get(key string) string {
	if key == "traceparent" {
		return envTp
	}

	return ""
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

// Clear sets the traceparent to empty string. Mainly for testing.
func (ec OtelCliCarrier) Clear() {
	envTp = ""
}

// loadTraceparent checks the environment first for TRACEPARENT then if filename
// isn't empty, it will read that file and look for a bare traceparent in that
// file.
func loadTraceparent(ctx context.Context, filename string) context.Context {
	ctx = loadTraceparentFromEnv(ctx)
	if filename != "" {
		ctx = loadTraceparentFromFile(ctx, filename)
	}

	if traceparentRequired {
		tp := getTraceparent(ctx) // get the text representation in the context
		if len(tp) > 0 && checkTracecarrierRe.MatchString(tp) {
			parts := strings.Split(tp, "-") // e.g. 00-9765b2f71c68b04dc0ad2a4d73027d6f-1881444346b6296e-01
			// return from here if everything looks ok, otherwise fall through to the log.Fatal
			if len(parts) > 3 && parts[1] != "00000000000000000000000000000000" && parts[2] != "0000000000000000" {
				return ctx
			}
		}

		log.Fatalf("failed to find a valid traceparent carrier in either environment for file '%s' while it's required by --tp-required", filename)
	}

	return ctx
}

// loadTraceparentFromFile reads a traceparent from filename and returns a
// context with the traceparent set. The format for the file as written is
// just a bare traceparent string. Whitespace, "export " and "TRACEPARENT=" are
// stripped automatically so the file can also be a valid shell snippet.
func loadTraceparentFromFile(ctx context.Context, filename string) context.Context {
	file, err := os.Open(filename)
	if err != nil {
		// only fatal when the tp carrier file is required explicitly, otherwise
		// just silently return the unmodified context
		if traceparentRequired {
			log.Fatalf("could not open file '%s' for read: %s", filename, err)
		} else {
			return ctx
		}
	}

	data, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatalf("failure while reading from file '%s': %s", filename, err)
	}

	tp := bytes.TrimSpace(data)
	if len(tp) == 0 {
		return ctx
	}

	// also accept 'export TRACEPARENT=' and 'TRACEPARENT='
	tp = bytes.TrimPrefix(tp, []byte("export "))
	tp = bytes.TrimPrefix(tp, []byte("TRACEPARENT="))

	if !checkTracecarrierRe.Match(tp) {
		// I have a doubt: should this be a soft failure?
		log.Fatalf("file '%s' was read but does not contain a valid traceparent", filename)
	}

	carrier := NewOtelCliCarrier()
	carrier.Set("traceparent", string(tp))

	prop := otel.GetTextMapPropagator()

	return prop.Extract(ctx, carrier)
}

// saveTraceparentToFile takes a context and filename and writes the tp from
// that context into the specified file.
func saveTraceparentToFile(ctx context.Context, filename string) {
	if filename == "" {
		return
	}

	tp := getTraceparent(ctx)

	err := ioutil.WriteFile(filename, []byte(tp), 0600)
	if err != nil {
		log.Fatalf("failure while writing to file '%s': %s", filename, err)
	}
}

// loadTraceparentFromEnv loads the traceparent from the environment variable
// TRACEPARENT and sets it in the returned Go context.
func loadTraceparentFromEnv(ctx context.Context) context.Context {
	// don't load the envvar when --tp-ignore-env is set
	if traceparentIgnoreEnv {
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
