package otelcli

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
		Fail:                   false,
		StatusCode:             "unset",
		StatusDescription:      "",
	}
}

// Config stores the runtime configuration for otel-cli.
// This is used as a singleton as "config" and accessed from many other files.
// Data structure is public so that it can serialize to json easily.
// mapstructure tags are for viper configuration unmarshaling, for config files and envvars.
type Config struct {
	Endpoint    string            `json:"endpoint" mapstructure:"endpoint"`
	Timeout     string            `json:"timeout" mapstructure:"timeout"`
	Headers     map[string]string `json:"headers" mapstructure:"otlp-headers"` // TODO: needs json marshaler hook to mask tokens
	Insecure    bool              `json:"insecure" mapstructure:"insecure"`
	Blocking    bool              `json:"blocking" mapstructure:"otlp-blocking"`
	NoTlsVerify bool              `json:"no_tls_verify" mapstructure:"no-tls-verify"`

	ServiceName       string            `json:"service_name" mapstructure:"service"`
	SpanName          string            `json:"span_name" mapstructure:"name"`
	Kind              string            `json:"span_kind" mapstructure:"kind"`
	Attributes        map[string]string `json:"span_attributes" mapstructure:"attrs"`
	StatusCode        string            `json:"span_status_code" mapstructure:"status-code"`
	StatusDescription string            `json:"span_status_description" mapstructure:"status-description"`

	TraceparentCarrierFile string `json:"traceparent_carrier_file" mapstructure:"tp-carrier"`
	TraceparentIgnoreEnv   bool   `json:"traceparent_ignore_env" mapstructure:"tp-ignore-env"`
	TraceparentPrint       bool   `json:"traceparent_print" mapstructure:"tp-print"`
	TraceparentPrintExport bool   `json:"traceparent_print_export" mapstructure:"tp-export"`
	TraceparentRequired    bool   `json:"traceparent_required" mapstructure:"tp-required"`

	BackgroundParentPollMs int    `json:"background_parent_poll_ms" mapstructure:"bp-poll-ms"`
	BackgroundSockdir      string `json:"background_socket_directory" mapstructure:"sockdir"`
	BackgroundWait         bool   `json:"background_wait" mapstructure:"wait"`

	SpanStartTime string `json:"span_start_time" mapstructure:"start"`
	SpanEndTime   string `json:"span_end_time" mapstructure:"end"`
	EventName     string `json:"event_name" mapstructure:"name"`
	EventTime     string `json:"event_time" mapstructure:"time"`

	CfgFile string `json:"config_file" mapstructure:"config"`
	Verbose bool   `json:"verbose" mapstructure:"verbose"`
	Fail    bool   `json:"fail" mapstructure:"fail"`
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
		"span_status_code":            c.StatusCode,
		"span_status_description":     c.StatusDescription,
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

// WithEndpoint returns the config with Endpoint set to the provided value.
func (c Config) WithEndpoint(with string) Config {
	c.Endpoint = with
	return c
}

// WithTimeout returns the config with Timeout set to the provided value.
func (c Config) WithTimeout(with string) Config {
	c.Timeout = with
	return c
}

// WithHeades returns the config with Heades set to the provided value.
func (c Config) WithHeaders(with map[string]string) Config {
	c.Headers = with
	return c
}

// WithInsecure returns the config with Insecure set to the provided value.
func (c Config) WithInsecure(with bool) Config {
	c.Insecure = with
	return c
}

// WithBlocking returns the config with Blocking set to the provided value.
func (c Config) WithBlocking(with bool) Config {
	c.Blocking = with
	return c
}

// WithNoTlsVerify returns the config with NoTlsVerify set to the provided value.
func (c Config) WithNoTlsVerify(with bool) Config {
	c.NoTlsVerify = with
	return c
}

// WithServiceName returns the config with ServiceName set to the provided value.
func (c Config) WithServiceName(with string) Config {
	c.ServiceName = with
	return c
}

// WithSpanName returns the config with SpanName set to the provided value.
func (c Config) WithSpanName(with string) Config {
	c.SpanName = with
	return c
}

// WithKind returns the config with Kind set to the provided value.
func (c Config) WithKind(with string) Config {
	c.Kind = with
	return c
}

// WithAttributes returns the config with Attributes set to the provided value.
func (c Config) WithAttributes(with map[string]string) Config {
	c.Attributes = with
	return c
}

// WithStatusCode returns the config with StatusCode set to the provided value.
func (c Config) WithStatusCode(with string) Config {
	c.StatusCode = with
	return c
}

// WithStatusDescription returns the config with StatusDescription set to the provided value.
func (c Config) WithStatusDescription(with string) Config {
	c.StatusDescription = with
	return c
}

// WithTraceparentCarrierFile returns the config with TraceparentCarrierFile set to the provided value.
func (c Config) WithTraceparentCarrierFile(with string) Config {
	c.TraceparentCarrierFile = with
	return c
}

// WithTraceparentIgnoreEnv returns the config with TraceparentIgnoreEnv set to the provided value.
func (c Config) WithTraceparentIgnoreEnv(with bool) Config {
	c.TraceparentIgnoreEnv = with
	return c
}

// WithTraceparentPrint returns the config with TraceparentPrint set to the provided value.
func (c Config) WithTraceparentPrint(with bool) Config {
	c.TraceparentPrint = with
	return c
}

// WithTraceparentPrintExport returns the config with TraceparentPrintExport set to the provided value.
func (c Config) WithTraceparentPrintExport(with bool) Config {
	c.TraceparentPrintExport = with
	return c
}

// WithTraceparentRequired returns the config with TraceparentRequired set to the provided value.
func (c Config) WithTraceparentRequired(with bool) Config {
	c.TraceparentRequired = with
	return c
}

// WithBackgroundParentPollMs returns the config with BackgroundParentPollMs set to the provided value.
func (c Config) WithBackgroundParentPollMs(with int) Config {
	c.BackgroundParentPollMs = with
	return c
}

// WithBackgroundSockdir returns the config with BackgroundSockdir set to the provided value.
func (c Config) WithBackgroundSockdir(with string) Config {
	c.BackgroundSockdir = with
	return c
}

// WithBackgroundWait returns the config with BackgroundWait set to the provided value.
func (c Config) WithBackgroundWait(with bool) Config {
	c.BackgroundWait = with
	return c
}

// WithSpanStartTime returns the config with SpanStartTime set to the provided value.
func (c Config) WithSpanStartTime(with string) Config {
	c.SpanStartTime = with
	return c
}

// WithSpanEndTime returns the config with SpanEndTime set to the provided value.
func (c Config) WithSpanEndTime(with string) Config {
	c.SpanEndTime = with
	return c
}

// WithEventName returns the config with EventName set to the provided value.
func (c Config) WithEventName(with string) Config {
	c.EventName = with
	return c
}

// WithEventTIme returns the config with EventTIme set to the provided value.
func (c Config) WithEventTime(with string) Config {
	c.EventTime = with
	return c
}

// WithCfgFile returns the config with CfgFile set to the provided value.
func (c Config) WithCfgFile(with string) Config {
	c.CfgFile = with
	return c
}

// WithVerbose returns the config with Verbose set to the provided value.
func (c Config) WithVerbose(with bool) Config {
	c.Verbose = with
	return c
}
