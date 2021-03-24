package cmd

import (
	"context"
	"log"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpgrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv"
)

// initTracer sets up the OpenTelemetry plumbing so it's ready to use.
// Returns a func() that encapuslates clean shutdown.
// TODO(@tobert): this pass does not configure metrics at all, and that will
// probably be handy for some users of otel-cli
func initTracer() (context.Context, func()) {
	ctx := context.Background()

	// TODO: make this configurable
	// probably copy the Ruby otel envvars
	driver := otlpgrpc.NewDriver(
		otlpgrpc.WithInsecure(),                  // TODO: make configurable
		otlpgrpc.WithEndpoint("localhost:30080"), // TODO: make configurable
	)
	// ^^ examples usually show this with the grpc.WithBlock() dial option to make
	// the connection synchronous, but we don't want that and instead rely on
	// the shutdown methods to make sure everything is done by the time we exit

	otlpExp, err := otlp.NewExporter(ctx, driver)
	if err != nil {
		log.Fatalf("failed to configure OTLP exporter: %s", err)
	}

	// set the service name that will show up in tracing UIs
	resAttrs := resource.WithAttributes(semconv.ServiceNameKey.String(appName))
	res, err := resource.New(ctx, resAttrs)
	if err != nil {
		log.Fatalf("failed to create OpenTelemetry service name resource: %s", err)
	}

	// SSP sends all completed spans to the exporter immediately and that is
	// exactly what we want/need in this app
	// https://github.com/open-telemetry/opentelemetry-go/blob/main/sdk/trace/simple_span_processor.go
	ssp := sdktrace.NewSimpleSpanProcessor(otlpExp)

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

		err = otlpExp.Shutdown(ctx)
		if err != nil {
			log.Fatalf("shutdown of OpenTelemetry OTLP exporter failed: %s", err)
		}
	}
}

// cliAttrsToOtel grabs the attributes passed via -a or --attrs and returns
// them in an []attribute.KeyValue
func cliAttrsToOtel() []attribute.KeyValue {
	otAttrs := []attribute.KeyValue{}
	for k, v := range attributes {

		// try to parse as numbers, and fall through to string
		var av attribute.Value
		if i, err := strconv.ParseInt(v, 0, 64); err == nil {
			av = attribute.Int64Value(i)
		} else if f, err := strconv.ParseFloat(v, 64); err == nil {
			av = attribute.Float64Value(f)
		} else if b, err := strconv.ParseBool(v); err == nil {
			av = attribute.BoolValue(b)
		} else {
			av = attribute.StringValue(v)
		}

		akv := attribute.KeyValue{
			Key:   attribute.Key(k),
			Value: av,
		}

		otAttrs = append(otAttrs, akv)
	}

	return otAttrs
}
