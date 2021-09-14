package cmd

// global config used by all of otel-cli
var config Config

const defaultOtlpEndpoint = "localhost:4317"
const spanBgSockfilename = "otel-cli-background.sock"

// Config stores the runtime configuration for otel-cli.
// This is used as a singleton as "config" and accessed from many other files.
// Data structure is public so that it can serialize to json easily.
type Config struct {
	Endpoint    string            `json:"endpoint"`
	Timeout     string            `json:"timeout"`
	Headers     map[string]string `json:"headers"` // TODO: needs json marshaler hook to mask tokens
	Insecure    bool              `json:"insecure"`
	Blocking    bool              `json:"blocking"`
	NoTlsVerify bool              `json:"no_tls_verify"`

	ServiceName string            `json:"service_name"`
	SpanName    string            `json:"span_name"`
	Kind        string            `json:"span_kind"`
	Attributes  map[string]string `json:"span_attributes"`

	TraceparentCarrierFile string `json:"traceparent_carrier_file"`
	TraceparentIgnoreEnv   bool   `json:"traceparent_ignore_env"`
	TraceparentPrint       bool   `json:"traceparent_print"`
	TraceparentPrintExport bool   `json:"traceparent_print_export"`
	TraceparentRequired    bool   `json:"traceparent_required"`

	BackgroundParentPollMs int    `json:"background_parent_poll_ms"`
	BackgroundSockdir      string `json:"background_socket_directory"`
	BackgroundWait         bool   `json:"background_wait"`

	SpanStartTime string `json:"span_start_time"`
	SpanEndTime   string `json:"span_end_time"`
	EventName     string `json:"event_name"`
	EventTime     string `json:"event_time"`

	CfgFile string `json:"config_file"`
	Verbose bool   `json:"verbose"`
}
