package main

import (
	"context"
	"log"


	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout"
	"go.opentelemetry.io/otel/sdk/metric/controller/push"
	"go.opentelemetry.io/otel/sdk/metric/processor/basic"
	"go.opentelemetry.io/otel/sdk/metric/selector/simple"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func main() {
	// set up the stdout exporter
	stdoutExporter, err := stdout.NewExporter([]stdout.Option{
		stdout.WithQuantiles([]float64{0.5, 0.95, 0.99}),
		stdout.WithPrettyPrint(),
	}...)
	if err != nil {
		log.Fatalf("failed to initialize otel stdout exporter: %s", err)
	}

	bsp := sdktrace.NewBatchSpanProcessor(stdoutExporter)
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(bsp))
	defer func () { _ = tp.Shutdown(context.Background()) }()
	pusher := push.New(
		basic.New(
			simple.NewWithExactDistribution(),
			stdoutExporter,
		),
		stdoutExporter,
	)
	pusher.Start()
	defer pusher.Stop()
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(pusher.MeterProvider())
}
