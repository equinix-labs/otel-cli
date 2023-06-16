package otelcli

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// OTLPClient is an interface that allows for StartClient to return either
// gRPC or HTTP clients.
// matches the OTel collector's similar interface
type OTLPClient interface {
	Start(context.Context) error
	UploadTraces(context.Context, []*tracepb.ResourceSpans) error
	Stop(context.Context) error
}

// StartClient uses the Config to setup and start either a gRPC or HTTP client,
// and returns the OTLPClient interface to them.
func StartClient(ctx context.Context, config Config) (context.Context, OTLPClient) {
	if !config.IsRecording() {
		return ctx, nil
	}

	if config.Protocol != "" && config.Protocol != "grpc" && config.Protocol != "http/protobuf" {
		err := fmt.Errorf("invalid protocol setting %q", config.Protocol)
		Diag.Error = err.Error()
		config.SoftFail(err.Error())
	}

	endpointURL, _ := ParseEndpoint(config)

	var client OTLPClient
	if config.Protocol != "grpc" &&
		(strings.HasPrefix(config.Protocol, "http/") ||
			endpointURL.Scheme == "http" ||
			endpointURL.Scheme == "https") {
		client = NewHttpClient(config)
	} else {
		client = NewGrpcClient(config)
	}

	err := client.Start(ctx)
	if err != nil {
		Diag.Error = err.Error()
		config.SoftFail("Failed to start OTLP client: %s", err)
	}

	return ctx, client
}

// SendSpan connects to the OTLP server, sends the span, and disconnects.
func SendSpan(ctx context.Context, client OTLPClient, config Config, span *tracepb.Span) error {
	if !config.IsRecording() {
		return nil
	}

	resourceAttrs, err := resourceAttributes(ctx, config.ServiceName)
	config.SoftFailIfErr(err)

	rsps := []*tracepb.ResourceSpans{
		{
			Resource: &resourcepb.Resource{
				Attributes: resourceAttrs,
			},
			ScopeSpans: []*tracepb.ScopeSpans{{
				Scope: &commonpb.InstrumentationScope{
					Name:                   "github.com/equinix-labs/otel-cli",
					Version:                config.Version,
					Attributes:             []*commonpb.KeyValue{},
					DroppedAttributesCount: 0,
				},
				Spans:     []*tracepb.Span{span},
				SchemaUrl: semconv.SchemaURL,
			}},
			SchemaUrl: semconv.SchemaURL,
		},
	}

	err = client.UploadTraces(ctx, rsps)
	if err != nil {
		Diag.Error = err.Error()
		return err
	}

	err = client.Stop(ctx)
	if err != nil {
		Diag.Error = err.Error()
		return err
	}

	return nil
}

// ParseEndpoint takes the endpoint or signal endpoint, augments as needed
// (e.g. bare host:port for gRPC) and then parses as a URL.
// https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md#endpoint-urls-for-otlphttp
func ParseEndpoint(config Config) (*url.URL, string) {
	var endpoint, source string
	var epUrl *url.URL
	var err error

	// signal-specific configs get precedence over general endpoint per OTel spec
	if config.TracesEndpoint != "" {
		endpoint = config.TracesEndpoint
		source = "signal"
	} else if config.Endpoint != "" {
		endpoint = config.Endpoint
		source = "general"
	} else {
		config.SoftFail("no endpoint configuration available")
	}

	parts := strings.Split(endpoint, ":")
	// bare hostname? can only be grpc, prepend
	if len(parts) == 1 {
		epUrl, err = url.Parse("grpc://" + endpoint + ":4317")
		if err != nil {
			config.SoftFail("error parsing (assumed) gRPC bare host address '%s': %s", endpoint, err)
		}
	} else if len(parts) > 1 { // could be URI or host:port
		// actual URIs
		// grpc:// is only an otel-cli thing, maybe should drop it?
		if parts[0] == "grpc" || parts[0] == "http" || parts[0] == "https" {
			epUrl, err = url.Parse(endpoint)
			if err != nil {
				config.SoftFail("error parsing provided %s URI '%s': %s", source, endpoint, err)
			}
		} else {
			// gRPC host:port
			epUrl, err = url.Parse("grpc://" + endpoint)
			if err != nil {
				config.SoftFail("error parsing (assumed) gRPC host:port address '%s': %s", endpoint, err)
			}
		}
	}

	// Per spec, /v1/traces is the default, appended to any url passed
	// to the general endpoint
	if strings.HasPrefix(epUrl.Scheme, "http") && source != "signal" && !strings.HasSuffix(epUrl.Path, "/v1/traces") {
		epUrl.Path = path.Join(epUrl.Path, "/v1/traces")
	}

	Diag.EndpointSource = source
	Diag.Endpoint = epUrl.String()
	return epUrl, source
}

// tlsConfig evaluates otel-cli configuration and returns a tls.Config
// that can be used by grpc or https.
func tlsConfig(config Config) *tls.Config {
	tlsConfig := &tls.Config{}

	if config.TlsNoVerify {
		Diag.InsecureSkipVerify = true
		tlsConfig.InsecureSkipVerify = true
	}

	// puts the provided CA certificate into the root pool
	// when not provided, Go TLS will automatically load the system CA pool
	if config.TlsCACert != "" {
		data, err := os.ReadFile(config.TlsCACert)
		if err != nil {
			config.SoftFail("failed to load CA certificate: %s", err)
		}

		certpool := x509.NewCertPool()
		certpool.AppendCertsFromPEM(data)
		tlsConfig.RootCAs = certpool
	}

	// client certificate authentication
	if config.TlsClientCert != "" && config.TlsClientKey != "" {
		clientPEM, err := os.ReadFile(config.TlsClientCert)
		if err != nil {
			config.SoftFail("failed to read client certificate file %s: %s", config.TlsClientCert, err)
		}
		clientKeyPEM, err := os.ReadFile(config.TlsClientKey)
		if err != nil {
			config.SoftFail("failed to read client key file %s: %s", config.TlsClientKey, err)
		}
		certPair, err := tls.X509KeyPair(clientPEM, clientKeyPEM)
		if err != nil {
			config.SoftFail("failed to parse client cert pair: %s", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certPair}
	} else if config.TlsClientCert != "" {
		config.SoftFail("client cert and key must be specified together")
	} else if config.TlsClientKey != "" {
		config.SoftFail("client cert and key must be specified together")
	}

	return tlsConfig
}

// deadlineCtx sets timeout on the context if the duration is non-zero.
// Otherwise it returns the context as-is.
func deadlineCtx(ctx context.Context, config Config, startupTime time.Time) (context.Context, context.CancelFunc) {
	if timeout := config.ParseCliTimeout(); timeout > 0 {
		deadline := startupTime.Add(timeout)
		return context.WithDeadline(ctx, deadline)
	}

	return ctx, func() {}
}

// isLoopbackAddr takes a url.URL, looks up the address, then returns true
// if it points at either a v4 or v6 loopback address.
// As I understood the OTLP spec, only host:port or an HTTP URL are acceptable.
// This function is _not_ meant to validate the endpoint, that will happen when
// otel-go attempts to connect to the endpoint.
func isLoopbackAddr(u *url.URL) (bool, error) {
	hostname := u.Hostname()

	if hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" {
		Diag.DetectedLocalhost = true
		return true, nil
	}

	ips, err := net.LookupIP(hostname)
	if err != nil {
		return false, fmt.Errorf("unable to look up hostname '%s': %s", hostname, err)
	}

	// all ips returned must be loopback to return true
	// cases where that isn't true should be super rare, and probably all shenanigans
	allAreLoopback := true
	for _, ip := range ips {
		if !ip.IsLoopback() {
			allAreLoopback = false
		}
	}

	Diag.DetectedLocalhost = allAreLoopback
	return allAreLoopback, nil
}

// isInsecureSchema returns true if the provided endpoint is an unencrypted HTTP URL or unix socket
func isInsecureSchema(endpoint string) bool {
	return strings.HasPrefix(endpoint, "http://") ||
		strings.HasPrefix(endpoint, "unix://")
}

// resourceAttributes calls the OTel SDK to get automatic resource attrs and
// returns them converted to []*commonpb.KeyValue for use with protobuf.
func resourceAttributes(ctx context.Context, serviceName string) ([]*commonpb.KeyValue, error) {
	// set the service name that will show up in tracing UIs
	resOpts := []resource.Option{
		resource.WithAttributes(semconv.ServiceNameKey.String(serviceName)),
		resource.WithFromEnv(), // maybe switch to manually loading this envvar?
		// TODO: make these autodetectors configurable
		//resource.WithHost(),
		//resource.WithOS(),
		//resource.WithProcess(),
		//resource.WithContainer(),
	}

	res, err := resource.New(ctx, resOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenTelemetry service name resource: %s", err)
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
			return nil, fmt.Errorf("BUG: unable to convert resource attribute, please file an issue")
		}

		ckv := commonpb.KeyValue{
			Key:   string(attr.Key),
			Value: av,
		}
		attrs = append(attrs, &ckv)
	}

	return attrs, nil
}

// retry calls the provided function and expects it to return (true, wait, err)
// to keep retrying, and (false, wait, err) to stop retrying and return.
// The wait value is a time.Duration so the server can recommend a backoff
// and it will be followed.
//
// This is a minimal retry mechanism that backs off linearly, 100ms at a time,
// up to a maximum of 5 seconds.
// While there are many robust implementations of retries out there, this one
// is just ~20 LoC and seems to work fine for otel-cli's modest needs. It should
// be rare for otel-cli to have a long timeout in the first place, and when it
// does, maybe it's ok to wait a few seconds.
// TODO: provide --otlp-retries (or something like that) option on CLI
// TODO: --otlp-retry-sleep? --otlp-retry-timeout?
// TODO: span events? hmm... feels weird to plumb spans this deep into the client
// but it's probably fine?
func retry(config Config, timeout time.Duration, fun retryFun) error {
	deadline := config.StartupTime.Add(timeout)
	sleep := time.Duration(0)
	for {
		if keepGoing, wait, err := fun(); err != nil {
			config.SoftLog("error on retry %d: %s", Diag.Retries, err)

			if keepGoing {
				if wait > 0 {
					if time.Now().Add(wait).After(deadline) {
						// wait will be after deadline, give up now
						return Diag.SetError(err)
					}
					time.Sleep(wait)
				} else {
					time.Sleep(sleep)
				}

				if time.Now().After(deadline) {
					return Diag.SetError(err)
				}

				// linearly increase sleep time up to 5 seconds
				if sleep < time.Second*5 {
					sleep = sleep + time.Millisecond*100
				}
			} else {
				return Diag.SetError(err)
			}
		} else {
			return nil
		}

		// It's retries instead of "tries" because "tries" means other things
		// too. Also, retries can default to 0 and it makes sense, saving
		// copying in test data.
		Diag.Retries++
	}
}

// retryFun is the function signature for functions passed to retry().
// Return (false, 0, err) to stop retrying. Return (true, 0, err) to continue
// retrying until timeout. Set the middle wait arg to a time.Duration to
// sleep a requested amount of time before next try
type retryFun func() (keepGoing bool, wait time.Duration, err error)
