// Package otlpclient implements a simple OTLP client, directly working with
// protobuf, gRPC, and net/http with minimal abstractions.
package otlpclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
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
type OTLPClient interface {
	Start(context.Context) (context.Context, error)
	UploadTraces(context.Context, []*tracepb.ResourceSpans) (context.Context, error)
	Stop(context.Context) (context.Context, error)
}

// TODO: rename to Config once the otelcli Config moves out
type OTLPConfig interface {
	GetTlsConfig() *tls.Config
	GetIsRecording() bool
	GetEndpoint() *url.URL
	GetInsecure() bool
	GetTimeout() time.Duration
	GetHeaders() map[string]string
	GetStartupTime() time.Time
	GetVersion() string
	GetServiceName() string
}

// SendSpan connects to the OTLP server, sends the span, and disconnects.
func SendSpan(ctx context.Context, client OTLPClient, config OTLPConfig, span *tracepb.Span) (context.Context, error) {
	if !config.GetIsRecording() {
		return ctx, nil
	}

	resourceAttrs, err := resourceAttributes(ctx, config.GetServiceName())
	if err != nil {
		return ctx, err
	}

	rsps := []*tracepb.ResourceSpans{
		{
			Resource: &resourcepb.Resource{
				Attributes: resourceAttrs,
			},
			ScopeSpans: []*tracepb.ScopeSpans{{
				Scope: &commonpb.InstrumentationScope{
					Name:                   "github.com/equinix-labs/otel-cli",
					Version:                config.GetVersion(),
					Attributes:             []*commonpb.KeyValue{},
					DroppedAttributesCount: 0,
				},
				Spans:     []*tracepb.Span{span},
				SchemaUrl: semconv.SchemaURL,
			}},
			SchemaUrl: semconv.SchemaURL,
		},
	}

	ctx, err = client.UploadTraces(ctx, rsps)
	if err != nil {
		return SaveError(ctx, time.Now(), err)
	}

	return ctx, nil
}

// deadlineCtx sets timeout on the context if the duration is non-zero.
// Otherwise it returns the context as-is.
func deadlineCtx(ctx context.Context, timeout time.Duration, startupTime time.Time) (context.Context, context.CancelFunc) {
	if timeout > 0 {
		deadline := startupTime.Add(timeout)
		return context.WithDeadline(ctx, deadline)
	}

	return ctx, func() {}
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

// otlpClientCtxKey is a type for storing otlp client information in context.Context safely.
type otlpClientCtxKey string

// TimestampedError is a timestamp + error string, to be stored in an ErrorList
type TimestampedError struct {
	Timestamp time.Time `json:"timestamp"`
	Error     string    `json:"error"`
}

// ErrorList is a list of TimestampedError
type ErrorList []TimestampedError

// errorListKey() returns the typed key used to store the error list in context.
func errorListKey() otlpClientCtxKey {
	return otlpClientCtxKey("otlp_errors")
}

// GetErrorList retrieves the error list from context and returns it. If the list
// is uninitialized, it initializes it in the returned context.
func GetErrorList(ctx context.Context) ErrorList {
	if cv := ctx.Value(errorListKey()); cv != nil {
		if l, ok := cv.(ErrorList); ok {
			return l
		} else {
			panic("BUG: failed to unwrap error list, please report an issue")
		}
	} else {
		return ErrorList{}
	}
}

// SaveError writes the provided error to the ErrorList in ctx, returning an
// updated ctx.
func SaveError(ctx context.Context, t time.Time, err error) (context.Context, error) {
	if err == nil {
		return ctx, nil
	}

	Diag.SetError(err) // legacy, will go away when Diag is removed

	te := TimestampedError{
		Timestamp: t,
		Error:     err.Error(),
	}

	errorList := GetErrorList(ctx)
	newList := append(errorList, te)
	ctx = context.WithValue(ctx, errorListKey(), newList)

	return ctx, err
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
func retry(ctx context.Context, config OTLPConfig, fun retryFun) (context.Context, error) {
	deadline := config.GetStartupTime().Add(config.GetTimeout())
	sleep := time.Duration(0)
	for {
		if ctx, keepGoing, wait, err := fun(ctx); err != nil {
			if err != nil {
				ctx, _ = SaveError(ctx, time.Now(), err)
			}
			//config.SoftLog("error on retry %d: %s", Diag.Retries, err)

			if keepGoing {
				if wait > 0 {
					if time.Now().Add(wait).After(deadline) {
						// wait will be after deadline, give up now
						return SaveError(ctx, time.Now(), err)
					}
					time.Sleep(wait)
				} else {
					time.Sleep(sleep)
				}

				if time.Now().After(deadline) {
					return SaveError(ctx, time.Now(), err)
				}

				// linearly increase sleep time up to 5 seconds
				if sleep < time.Second*5 {
					sleep = sleep + time.Millisecond*100
				}
			} else {
				return SaveError(ctx, time.Now(), err)
			}
		} else {
			return ctx, nil
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
type retryFun func(ctx context.Context) (ctxOut context.Context, keepGoing bool, wait time.Duration, err error)
