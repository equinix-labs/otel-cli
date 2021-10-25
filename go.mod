module github.com/equinix-labs/otel-cli

go 1.14

require (
	github.com/google/go-cmp v0.5.6
	github.com/mitchellh/go-homedir v1.1.0
	github.com/pterm/pterm v0.12.30
	github.com/spf13/cobra v1.1.3
	github.com/spf13/viper v1.7.0
	go.opentelemetry.io/otel v1.0.1
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.0.1
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.0.1
	go.opentelemetry.io/otel/sdk v1.0.1
	go.opentelemetry.io/otel/trace v1.0.1
	go.opentelemetry.io/proto/otlp v0.9.0
	google.golang.org/grpc v1.41.0
)
