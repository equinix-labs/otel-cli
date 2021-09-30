package cmd

import (
	"encoding/json"
	"strconv"
)

// config is the global configuraiton used by all of otel-cli.
// It is written to by Cobra and Viper.
var config Config

// defaults is a Config set to default values used by Cobra
var defaults = DefaultConfig()

const defaultOtlpEndpoint = "localhost:4317"
const spanBgSockfilename = "otel-cli-background.sock"

// DefaultConfig returns a Config with all defaults set.
func DefaultConfig() Config {
	return Config{
		Endpoint:               "",
		Timeout:                "1s",
		Headers:                map[string]string{},
		Insecure:               false,
		Blocking:               false,
		NoTlsVerify:            false,
		ServiceName:            "otel-cli",
		SpanName:               "todo-generate-default-span-names",
		Kind:                   "client",
		Attributes:             map[string]string{},
		TraceparentCarrierFile: "",
		TraceparentIgnoreEnv:   false,
		TraceparentPrint:       false,
		TraceparentPrintExport: false,
		TraceparentRequired:    false,
		BackgroundParentPollMs: 10,
		BackgroundSockdir:      "",
		BackgroundWait:         false,
		SpanStartTime:          "now",
		SpanEndTime:            "now",
		EventName:              "todo-generate-default-event-names",
		EventTime:              "now",
		CfgFile:                "",
		Verbose:                false,
	}
}

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

// UnmarshalJSON makes sure that any Config loaded from JSON has its default
// values set. This is critical to comparisons in the otel-cli test suite.
func (c *Config) UnmarshalJSON(js []byte) error {
	// use a type alias to avoid recursion on Unmarshaler
	type config Config
	defaults := config(DefaultConfig())
	if err := json.Unmarshal(js, &defaults); err != nil {
		return err
	}
	*c = Config(defaults)
	return nil
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
