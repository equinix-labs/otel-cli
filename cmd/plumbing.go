package cmd

import (
	"context"
	"crypto/tls"
	"log"
	"net"
	"net/url"
	"regexp"
	"strings"

	"go.opentelemetry.io/otel"
	otlpgrpc "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	stdout "go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// initTracer sets up the OpenTelemetry plumbing so it's ready to use.
// Returns a context and a func() that encapuslates clean shutdown.
//
func initTracer() (context.Context, func()) {
	ctx := context.Background()

	grpcOpts := []otlpgrpc.Option{otlpgrpc.WithEndpoint(otlpEndpoint)}

	// gRPC does the right thing and forces us to say WithInsecure to disable encryption,
	// but I expect most users of this program to point at a localhost endpoint that might not
	// have any encryption available, or setting it up raises the bar of entry too high.
	// The compromise is to automatically flip this flag to true when endpoint contains an
	// an obvious "localhost", "127.0.0.x", or "::1" address.
	if otlpInsecure || isLoopbackAddr(otlpEndpoint) {
		grpcOpts = append(grpcOpts, otlpgrpc.WithInsecure())
	} else {
		var config *tls.Config
		if noTlsVerify {
			config = &tls.Config{
				InsecureSkipVerify: true,
			}
		}
		grpcOpts = append(grpcOpts, otlpgrpc.WithDialOption(grpc.WithTransportCredentials(credentials.NewTLS(config))))
	}

	// support for OTLP headers, e.g. for authenticating to SaaS OTLP endpoints
	if len(otlpHeaders) > 0 {
		// fortunately WithHeaders can accept the string map as-is
		grpcOpts = append(grpcOpts, otlpgrpc.WithHeaders(otlpHeaders))
	}

	// OTLP examples usually show this with the grpc.WithBlock() dial option to
	// make the connection synchronous, but it's not the right default for cli
	// instead, rely on the shutdown methods to make sure everything is flushed
	// by the time the program exits.
	if otlpBlocking {
		grpcOpts = append(grpcOpts, otlpgrpc.WithDialOption(grpc.WithBlock()))
	}

	var exporter sdktrace.SpanExporter // allows overwrite in --test mode
	exporter, err := otlpgrpc.New(ctx, grpcOpts...)
	if err != nil {
		log.Fatalf("failed to configure OTLP exporter: %s", err)
	}

	// when in test mode, let the otlp exporter setup happen, then overwrite it
	// with the stdout exporter so spans only go to stdout
	if testMode {
		exporter, err = stdout.New()
		if err != nil {
			log.Fatalf("failed to configure stdout exporter in --test mode: %s", err)
		}
	}

	// set the service name that will show up in tracing UIs
	resAttrs := resource.WithAttributes(semconv.ServiceNameKey.String(serviceName))
	res, err := resource.New(ctx, resAttrs)
	if err != nil {
		log.Fatalf("failed to create OpenTelemetry service name resource: %s", err)
	}

	// SSP sends all completed spans to the exporter immediately and that is
	// exactly what we want/need in this app
	// https://github.com/open-telemetry/opentelemetry-go/blob/main/sdk/trace/simple_span_processor.go
	ssp := sdktrace.NewSimpleSpanProcessor(exporter)

	// ParentBased/AlwaysSample Sampler is the default and that's fine for this
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(ssp),
	)

	// inject the tracer into the otel globals (and this starts the background stuff, I think)
	otel.SetTracerProvider(tracerProvider)

	// set up the W3C trace context as the global propagator
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// callers need to defer this to make sure all the data gets flushed out
	return ctx, func() {
		err = tracerProvider.Shutdown(ctx)
		if err != nil {
			log.Fatalf("shutdown of OpenTelemetry tracerProvider failed: %s", err)
		}

		err = exporter.Shutdown(ctx)
		if err != nil {
			log.Fatalf("shutdown of OpenTelemetry OTLP exporter failed: %s", err)
		}
	}
}

// isLoopbackAddr takes a dial string, looks up the address, then returns true
// if it points at either a v4 or v6 loopback address.
// As I understood the OTLP spec, only host:port or an HTTP URL are acceptable.
// This function is _not_ meant to validate the endpoint, that will happen when
// otel-go attempts to connect to the endpoint as a gRPC dial address.
func isLoopbackAddr(endpoint string) bool {
	hpRe := regexp.MustCompile(`^[\w.-]+:\d+$`)
	uriRe := regexp.MustCompile(`^(http|https)`)

	endpoint = strings.TrimSpace(endpoint)

	var hostname string
	if hpRe.MatchString(endpoint) {
		parts := strings.SplitN(endpoint, ":", 2)
		hostname = parts[0]
	} else if uriRe.MatchString(endpoint) {
		u, err := url.Parse(endpoint)
		if err != nil {
			log.Fatalf("error parsing provided URI '%s': %s", endpoint, err)
		}
		hostname = u.Hostname()
	} else {
		log.Fatalf("'%s' is not a valid endpoint, must be host:port or a URI", endpoint)
	}

	ips, err := net.LookupIP(hostname)
	if err != nil {
		log.Fatalf("unable to look up hostname '%s': %s", hostname, err)
	}

	// all ips returned must be loopback to return true
	// cases where that isn't true should be super rare, and probably all shenanigans
	allAreLoopback := true
	for _, ip := range ips {
		if !ip.IsLoopback() {
			allAreLoopback = false
		}
	}

	return allAreLoopback
}
