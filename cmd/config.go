package cmd

import "strconv"

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

// ToStringMap flattens the configuration into a stringmap that is easy to work
// with in tests especially with cmp.Diff. See test_main.go.
func (c Config) ToStringMap() map[string]string {
	return map[string]string{
		"endpoint":                    c.Endpoint,
		"timeout":                     c.Timeout,
		"headers":                     flattenStringMap(c.Headers, "{}"),
		"insecure":                    strconv.FormatBool(c.Insecure),
		"blocking":                    strconv.FormatBool(c.Blocking),
		"no_tls_verify":               strconv.FormatBool(c.NoTlsVerify),
		"service_name":                c.ServiceName,
		"span_name":                   c.SpanName,
		"span_kind":                   c.Kind,
		"span_attributes":             flattenStringMap(c.Attributes, "{}"),
		"traceparent_carrier_file":    c.TraceparentCarrierFile,
		"traceparent_ignore_env":      strconv.FormatBool(c.TraceparentIgnoreEnv),
		"traceparent_print":           strconv.FormatBool(c.TraceparentPrint),
		"traceparent_print_export":    strconv.FormatBool(c.TraceparentPrintExport),
		"traceparent_required":        strconv.FormatBool(c.TraceparentRequired),
		"background_parent_poll_ms":   strconv.Itoa(c.BackgroundParentPollMs),
		"background_socket_directory": c.BackgroundSockdir,
		"background_wait":             strconv.FormatBool(c.BackgroundWait),
		"span_start_time":             c.SpanStartTime,
		"span_end_time":               c.SpanEndTime,
		"event_name":                  c.EventName,
		"event_time":                  c.EventTime,
		"config_file":                 c.CfgFile,
		"verbose":                     strconv.FormatBool(c.Verbose),
	}
}
