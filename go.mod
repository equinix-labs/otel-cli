module github.com/equinix-labs/otel-cli

go 1.14

require (
	github.com/google/go-cmp v0.5.9
	github.com/pkg/errors v0.8.1
	github.com/pterm/pterm v0.12.53
	github.com/spf13/cobra v1.6.1
	go.opentelemetry.io/otel v1.4.1
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.4.1
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.4.1
	go.opentelemetry.io/otel/sdk v1.4.1
	go.opentelemetry.io/otel/trace v1.4.1
	go.opentelemetry.io/proto/otlp v0.12.0
	google.golang.org/grpc v1.44.0
)
