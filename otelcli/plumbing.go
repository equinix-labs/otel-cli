package otelcli

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// SendSpan connects to the OTLP server, sends the span, and disconnects.
func SendSpan(ctx context.Context, span tracepb.Span) error {
	if !config.IsRecording() {
		return nil
	}

	if config.Protocol != "" && config.Protocol != "grpc" && config.Protocol != "http/protobuf" {
		err := fmt.Errorf("invalid protocol setting %q", config.Protocol)
		diagnostics.OtelError = err.Error()
		return err
	}

	var client otlptrace.Client
	if config.Protocol != "grpc" &&
		(strings.HasPrefix(config.Protocol, "http/") ||
			strings.HasPrefix(config.Endpoint, "http://") ||
			strings.HasPrefix(config.Endpoint, "https://")) {
		client = otlptracehttp.NewClient(httpOptions()...)
	} else {
		client = otlptracegrpc.NewClient(grpcOptions()...)
	}

	err := client.Start(ctx)
	if err != nil {
		diagnostics.OtelError = err.Error()
		return err
	}

	rsps := []*tracepb.ResourceSpans{
		{
			Resource: &resourcepb.Resource{
				Attributes: resourceAttributes(ctx),
			},
			ScopeSpans: []*tracepb.ScopeSpans{{
				Scope: &commonpb.InstrumentationScope{
					Name:                   "github.com/equinix-labs/otel-cli",
					Version:                rootCmd.Version, // TODO: plumb this through config
					Attributes:             []*commonpb.KeyValue{},
					DroppedAttributesCount: 0,
				},
				Spans:     []*tracepb.Span{&span},
				SchemaUrl: semconv.SchemaURL,
			}},
			SchemaUrl: semconv.SchemaURL,
		},
	}

	err = client.UploadTraces(ctx, rsps)
	if err != nil {
		diagnostics.OtelError = err.Error()
		return err
	}

	err = client.Stop(ctx)
	if err != nil {
		diagnostics.OtelError = err.Error()
		return err
	}

	return nil
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
func grpcOptions() []otlptracegrpc.Option {
	grpcOpts := []otlptracegrpc.Option{}

	// per comment in initTracer(), grpc:// is specific to otel-cli
	if strings.HasPrefix(config.Endpoint, "grpc://") ||
		strings.HasPrefix(config.Endpoint, "http://") ||
		strings.HasPrefix(config.Endpoint, "https://") {
		ep, err := url.Parse(config.Endpoint)
		if err != nil {
			softFail("error parsing provided gRPC URI '%s': %s", config.Endpoint, err)
		}
		grpcOpts = append(grpcOpts, otlptracegrpc.WithEndpoint(ep.Hostname()+":"+ep.Port()))
	} else {
		grpcOpts = append(grpcOpts, otlptracegrpc.WithEndpoint(config.Endpoint))
	}

	// set timeout if the duration is non-zero, otherwise just leave things to the defaults
	if timeout := parseCliTimeout(); timeout > 0 {
		grpcOpts = append(grpcOpts, otlptracegrpc.WithTimeout(timeout))
	}

	// gRPC does the right thing and forces us to say WithInsecure to disable encryption,
	// but I expect most users of this program to point at a localhost endpoint that might not
	// have any encryption available, or setting it up raises the bar of entry too high.
	// The compromise is to automatically flip this flag to true when endpoint contains an
	// an obvious "localhost", "127.0.0.x", or "::1" address.
	if config.Insecure || (isLoopbackAddr(config.Endpoint) && !strings.HasPrefix(config.Endpoint, "https")) {
		grpcOpts = append(grpcOpts, otlptracegrpc.WithInsecure())
	} else if !isInsecureSchema(config.Endpoint) {
		grpcOpts = append(grpcOpts, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(tlsConfig())))
	}

	// support for OTLP headers, e.g. for authenticating to SaaS OTLP endpoints
	if len(config.Headers) > 0 {
		// fortunately WithHeaders can accept the string map as-is
		grpcOpts = append(grpcOpts, otlptracegrpc.WithHeaders(config.Headers))
	}

	// OTLP examples usually show this with the grpc.WithBlock() dial option to
	// make the connection synchronous, but it's not the right default for cli
	// instead, rely on the shutdown methods to make sure everything is flushed
	// by the time the program exits.
	if config.Blocking {
		grpcOpts = append(grpcOpts, otlptracegrpc.WithDialOption(grpc.WithBlock()))
	}

	return grpcOpts
}

// httpOptions converts config to an otlphttp.Option list.
func httpOptions() []otlptracehttp.Option {
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
	httpOpts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(endpointHostAndPort)}

	if endpointURL.Path == "" {
		// Per spec, /v1/traces is the default:
		// (https://github.com/open-telemetry/opentelemetry-specification/blob/c14f5416605cb1bfce6e6e1984cbbeceb1bf35a2/specification/protocol/exporter.md#endpoint-urls-for-otlphttp)
		endpointURL.Path = "/v1/traces"
	}

	httpOpts = append(httpOpts, otlptracehttp.WithURLPath(endpointURL.Path))

	// set timeout if the duration is non-zero, otherwise just leave things to the defaults
	if timeout := parseCliTimeout(); timeout > 0 {
		httpOpts = append(httpOpts, otlptracehttp.WithTimeout(timeout))
	}

	// otlptracehttp does the right thing and forces us to say WithInsecure to disable
	// encryption, but I expect most users of this program to point at a localhost
	// endpoint that might not have any encryption available, or setting it up
	// raises the bar of entry too high.  The compromise is to automatically flip
	// this flag to true when endpoint contains an an obvious "localhost",
	// "127.0.0.x", or "::1" address.
	if config.Insecure || (isLoopbackAddr(config.Endpoint) && !strings.HasPrefix(config.Endpoint, "https")) {
		httpOpts = append(httpOpts, otlptracehttp.WithInsecure())
	} else if !isInsecureSchema(config.Endpoint) {
		httpOpts = append(httpOpts, otlptracehttp.WithTLSClientConfig(tlsConfig()))
	}

	// support for OTLP headers, e.g. for authenticating to SaaS OTLP endpoints
	if len(config.Headers) > 0 {
		// fortunately WithHeaders can accept the string map as-is
		httpOpts = append(httpOpts, otlptracehttp.WithHeaders(config.Headers))
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

// resourceAttributes calls the OTel SDK to get automatic resource attrs and
// returns them converted to []*commonpb.KeyValue for use with protobuf.
func resourceAttributes(ctx context.Context) []*commonpb.KeyValue {
	// set the service name that will show up in tracing UIs
	resAttrs := resource.WithAttributes(semconv.ServiceNameKey.String(config.ServiceName))
	res, err := resource.New(ctx, resAttrs)
	if err != nil {
		softFail("failed to create OpenTelemetry service name resource: %s", err)
	}

	attrs := []*commonpb.KeyValue{}

	for _, attr := range res.Attributes() {
		av := new(commonpb.AnyValue)

		// does not implement slice types... should be fine?
		switch attr.Value.Type() {
		case attribute.BOOL:
			av.Value = &commonpb.AnyValue_BoolValue{BoolValue: attr.Value.AsBool()}
		case attribute.INT64:
			av.Value = &commonpb.AnyValue_IntValue{IntValue: attr.Value.AsInt64()}
		case attribute.FLOAT64:
			av.Value = &commonpb.AnyValue_DoubleValue{DoubleValue: attr.Value.AsFloat64()}
		case attribute.STRING:
			av.Value = &commonpb.AnyValue_StringValue{StringValue: attr.Value.AsString()}
		default:
			softFail("BUG: unable to convert resource attribute, please file an issue")
		}

		ckv := commonpb.KeyValue{
			Key:   string(attr.Key),
			Value: av,
		}
		attrs = append(attrs, &ckv)
	}

	return attrs
}
