package otelcli

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/url"
	"os"
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

	if config.Protocol != "" && config.Protocol != "grpc" && config.Protocol != "http/protobuf" {
		softFail("invalid protocol setting %q", config.Protocol)
	}

	var exporter sdktrace.SpanExporter // allows overwrite in --test mode
	var err error

	// The OTel spec does not support grpc:// or any way to deterministically demand
	// gRPC via the endpoint URI, preferring OTEL_EXPORTER_PROTOCOL to do so. This is
	// awkward for otel-cli so we break with the spec. otel-cli will only resolve
	// http(s):// to HTTP protocols, defaults bare host:port to gRPC, and supports
	// grpc:// to definitely use gRPC to connect out.
	if config.Protocol != "grpc" &&
		(strings.HasPrefix(config.Protocol, "http/") ||
			strings.HasPrefix(config.Endpoint, "http://") ||
			strings.HasPrefix(config.Endpoint, "https://")) {
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

// tlsConfig evaluates otel-cli configuration and returns a tls.Config
// that can be used by grpc or https.
func tlsConfig() *tls.Config {
	tlsConfig := &tls.Config{}

	if config.TlsNoVerify {
		diagnostics.InsecureSkipVerify = true
		tlsConfig.InsecureSkipVerify = true
	}

	// puts the provided CA certificate into the root pool
	// when not provided, Go TLS will automatically load the system CA pool
	if config.TlsCACert != "" {
		data, err := os.ReadFile(config.TlsCACert)
		if err != nil {
			softFail("failed to load CA certificate: %s", err)
		}

		certpool := x509.NewCertPool()
		certpool.AppendCertsFromPEM(data)
		tlsConfig.RootCAs = certpool
	}

	// client certificate authentication
	if config.TlsClientCert != "" && config.TlsClientKey != "" {
		clientPEM, err := os.ReadFile(config.TlsClientCert)
		if err != nil {
			softFail("failed to read client certificate file %s: %s", config.TlsClientCert, err)
		}
		clientKeyPEM, err := os.ReadFile(config.TlsClientKey)
		if err != nil {
			softFail("failed to read client key file %s: %s", config.TlsClientKey, err)
		}
		certPair, err := tls.X509KeyPair(clientPEM, clientKeyPEM)
		if err != nil {
			softFail("failed to parse client cert pair: %s", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certPair}
	} else if config.TlsClientCert != "" {
		softFail("client cert and key must be specified together")
	} else if config.TlsClientKey != "" {
		softFail("client cert and key must be specified together")
	}

	return tlsConfig
}

// grpcOptions convets config settings to an otlpgrpc.Option list.
func grpcOptions() []otlpgrpc.Option {
	grpcOpts := []otlpgrpc.Option{}

	// per comment in initTracer(), grpc:// is specific to otel-cli
	if strings.HasPrefix(config.Endpoint, "grpc://") ||
		strings.HasPrefix(config.Endpoint, "http://") ||
		strings.HasPrefix(config.Endpoint, "https://") {
		ep, err := url.Parse(config.Endpoint)
		if err != nil {
			softFail("error parsing provided gRPC URI '%s': %s", config.Endpoint, err)
		}
		grpcOpts = append(grpcOpts, otlpgrpc.WithEndpoint(ep.Hostname()+":"+ep.Port()))
	} else {
		grpcOpts = append(grpcOpts, otlpgrpc.WithEndpoint(config.Endpoint))
	}

	// set timeout if the duration is non-zero, otherwise just leave things to the defaults
	if timeout := parseCliTimeout(); timeout > 0 {
		grpcOpts = append(grpcOpts, otlpgrpc.WithTimeout(timeout))
	}

	// gRPC does the right thing and forces us to say WithInsecure to disable encryption,
	// but I expect most users of this program to point at a localhost endpoint that might not
	// have any encryption available, or setting it up raises the bar of entry too high.
	// The compromise is to automatically flip this flag to true when endpoint contains an
	// an obvious "localhost", "127.0.0.x", or "::1" address.
	if config.Insecure || (isLoopbackAddr(config.Endpoint) && !strings.HasPrefix(config.Endpoint, "https")) {
		grpcOpts = append(grpcOpts, otlpgrpc.WithInsecure())
	} else if !isInsecureSchema(config.Endpoint) {
		grpcOpts = append(grpcOpts, otlpgrpc.WithTLSCredentials(credentials.NewTLS(tlsConfig())))
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

// httpOptions converts config to an otlphttp.Option list.
func httpOptions() []otlphttp.Option {
	endpointURL, err := url.Parse(config.Endpoint)
	if err != nil {
		softFail("error parsing provided HTTP URI '%s': %s", config.Endpoint, err)
	}

	var endpointHostAndPort = endpointURL.Host
	if endpointURL.Port() == "" {
		if endpointURL.Scheme == "https" {
			endpointHostAndPort += ":443"
		} else {
			endpointHostAndPort += ":80"
		}
	}
	httpOpts := []otlphttp.Option{otlphttp.WithEndpoint(endpointHostAndPort)}

	if endpointURL.Path == "" {
		// Per spec, /v1/traces is the default:
		// (https://github.com/open-telemetry/opentelemetry-specification/blob/c14f5416605cb1bfce6e6e1984cbbeceb1bf35a2/specification/protocol/exporter.md#endpoint-urls-for-otlphttp)
		endpointURL.Path = "/v1/traces"
	}

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
	if config.Insecure || (isLoopbackAddr(config.Endpoint) && !strings.HasPrefix(config.Endpoint, "https")) {
		httpOpts = append(httpOpts, otlphttp.WithInsecure())
	} else if !isInsecureSchema(config.Endpoint) {
		httpOpts = append(httpOpts, otlphttp.WithTLSClientConfig(tlsConfig()))
	}

	// support for OTLP headers, e.g. for authenticating to SaaS OTLP endpoints
	if len(config.Headers) > 0 {
		// fortunately WithHeaders can accept the string map as-is
		httpOpts = append(httpOpts, otlphttp.WithHeaders(config.Headers))
	}

	return httpOpts
}

// isLoopbackAddr takes a dial string, looks up the address, then returns true
// if it points at either a v4 or v6 loopback address.
// As I understood the OTLP spec, only host:port or an HTTP URL are acceptable.
// This function is _not_ meant to validate the endpoint, that will happen when
// otel-go attempts to connect to the endpoint.
func isLoopbackAddr(endpoint string) bool {
	hpRe := regexp.MustCompile(`^[\w\.-]+:\d+$`)
	uriRe := regexp.MustCompile(`^(grpc|http|https):`)

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
	} else if strings.HasPrefix(endpoint, "unix://") {
		return false
	} else {
		softFail("'%s' is not a valid endpoint, must be host:port or a URI", endpoint)
	}

	if hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" {
		diagnostics.DetectedLocalhost = true
		return true
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

// isInsecureSchema returns true if the provided endpoint is an unencrypted HTTP URL or unix socket
func isInsecureSchema(endpoint string) bool {
	return strings.HasPrefix(endpoint, "http://") ||
		strings.HasPrefix(endpoint, "unix://")
}
