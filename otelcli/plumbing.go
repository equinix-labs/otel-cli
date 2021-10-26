package otelcli

import (
	"context"
	"crypto/tls"
	"net"
	"net/url"
	"regexp"
	"strings"

	"go.opentelemetry.io/otel"
	otlpgrpc "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otlphttp "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func grpcOptions() []otlpgrpc.Option {
	grpcOpts := []otlpgrpc.Option{otlpgrpc.WithEndpoint(config.Endpoint)}

	// set timeout if the duration is non-zero, otherwise just leave things to the defaults
	if timeout := parseCliTimeout(); timeout > 0 {
		grpcOpts = append(grpcOpts, otlpgrpc.WithTimeout(timeout))
	}

	// gRPC does the right thing and forces us to say WithInsecure to disable encryption,
	// but I expect most users of this program to point at a localhost endpoint that might not
	// have any encryption available, or setting it up raises the bar of entry too high.
	// The compromise is to automatically flip this flag to true when endpoint contains an
	// an obvious "localhost", "127.0.0.x", or "::1" address.
	if config.Insecure || isLoopbackAddr(config.Endpoint) {
		grpcOpts = append(grpcOpts, otlpgrpc.WithInsecure())
	} else {
		var tlsConfig *tls.Config
		if config.NoTlsVerify {
			tlsConfig = &tls.Config{
				InsecureSkipVerify: true,
			}
		}
		grpcOpts = append(grpcOpts, otlpgrpc.WithDialOption(grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))))
	}

	// support for OTLP headers, e.g. for authenticating to SaaS OTLP endpoints
	if len(config.Headers) > 0 {
		// fortunately WithHeaders can accept the string map as-is
		grpcOpts = append(grpcOpts, otlpgrpc.WithHeaders(config.Headers))
	}

	// OTLP examples usually show this with the grpc.WithBlock() dial option to
	// make the connection synchronous, but it's not the right default for cli
	// instead, rely on the shutdown methods to make sure everything is flushed
	// by the time the program exits.
	if config.Blocking {
		grpcOpts = append(grpcOpts, otlpgrpc.WithDialOption(grpc.WithBlock()))
	}

	return grpcOpts
}

func httpOptions() []otlphttp.Option {
	endpointURL, _ := url.Parse(config.Endpoint)

	var endpointHostAndPort = endpointURL.Host
	if endpointURL.Port() == "" {
		if endpointURL.Scheme == "https" {
			endpointHostAndPort += ":443"
		} else {
			endpointHostAndPort += ":80"
		}
	}
	httpOpts := []otlphttp.Option{otlphttp.WithEndpoint(endpointHostAndPort)}

	httpOpts = append(httpOpts, otlphttp.WithURLPath(endpointURL.Path))

	// set timeout if the duration is non-zero, otherwise just leave things to the defaults
	if timeout := parseCliTimeout(); timeout > 0 {
		httpOpts = append(httpOpts, otlphttp.WithTimeout(timeout))
	}

	// otlphttp does the right thing and forces us to say WithInsecure to disable
	// encryption, but I expect most users of this program to point at a localhost
	// endpoint that might not have any encryption available, or setting it up
	// raises the bar of entry too high.  The compromise is to automatically flip
	// this flag to true when endpoint contains an an obvious "localhost",
	// "127.0.0.x", or "::1" address.
	if config.Insecure || isLoopbackAddr(config.Endpoint) {
		httpOpts = append(httpOpts, otlphttp.WithInsecure())
	} else {
		var tlsConfig *tls.Config
		if config.NoTlsVerify {
			tlsConfig = &tls.Config{
				InsecureSkipVerify: true,
			}
		}
		httpOpts = append(httpOpts, otlphttp.WithTLSClientConfig(tlsConfig))
	}

	// support for OTLP headers, e.g. for authenticating to SaaS OTLP endpoints
	if len(config.Headers) > 0 {
		// fortunately WithHeaders can accept the string map as-is
		httpOpts = append(httpOpts, otlphttp.WithHeaders(config.Headers))
	}

	return httpOpts
}

// initTracer sets up the OpenTelemetry plumbing so it's ready to use.
// Returns a context and a func() that encapuslates clean shutdown.
func initTracer() (context.Context, func()) {
	ctx := context.Background()

	otel.SetErrorHandler(diagnostics)

	// when no endpoint is set, do not set up plumbing. everything will still
	// run but in non-recording mode, and otel-cli is effectively disabled
	// and will not time out trying to connect out
	if config.Endpoint == "" {
		return ctx, func() {}
	}

	var exporter sdktrace.SpanExporter // allows overwrite in --test mode
	var err error

	if strings.HasPrefix(config.Endpoint, "http://") ||
		strings.HasPrefix(config.Endpoint, "https://") {
		exporter, err = otlphttp.New(ctx, httpOptions()...)
		if err != nil {
			softFail("failed to configure OTLP/HTTP exporter: %s", err)
		}
	} else {
		exporter, err = otlpgrpc.New(ctx, grpcOptions()...)
		if err != nil {
			softFail("failed to configure OTLP/GRPC exporter: %s", err)
		}
	}

	// set the service name that will show up in tracing UIs
	resAttrs := resource.WithAttributes(semconv.ServiceNameKey.String(config.ServiceName))
	res, err := resource.New(ctx, resAttrs)
	if err != nil {
		softFail("failed to create OpenTelemetry service name resource: %s", err)
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
			softFail("shutdown of OpenTelemetry tracerProvider failed: %s", err)
		}

		err = exporter.Shutdown(ctx)
		if err != nil {
			softFail("shutdown of OpenTelemetry OTLP exporter failed: %s", err)
		}
	}
}

// isLoopbackAddr takes a dial string, looks up the address, then returns true
// if it points at either a v4 or v6 loopback address.
// As I understood the OTLP spec, only host:port or an HTTP URL are acceptable.
// This function is _not_ meant to validate the endpoint, that will happen when
// otel-go attempts to connect to the endpoint.
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
			softFail("error parsing provided URI '%s': %s", endpoint, err)
		}
		hostname = u.Hostname()
	} else {
		softFail("'%s' is not a valid endpoint, must be host:port or a URI", endpoint)
	}

	ips, err := net.LookupIP(hostname)
	if err != nil {
		softFail("unable to look up hostname '%s': %s", hostname, err)
	}

	// all ips returned must be loopback to return true
	// cases where that isn't true should be super rare, and probably all shenanigans
	allAreLoopback := true
	for _, ip := range ips {
		if !ip.IsLoopback() {
			allAreLoopback = false
		}
	}

	diagnostics.DetectedLocalhost = allAreLoopback
	return allAreLoopback
}
