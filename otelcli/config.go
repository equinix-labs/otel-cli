package otelcli

import (
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// config is the global configuraiton used by all of otel-cli.
// It is written to by Cobra and Viper.
var config Config

// defaults is a Config set to default values used by Cobra
var defaults = DefaultConfig()

const defaultOtlpEndpoint = "grpc://localhost:4317"
const spanBgSockfilename = "otel-cli-background.sock"

// DefaultConfig returns a Config with all defaults set.
func DefaultConfig() Config {
	return Config{
		Endpoint:               "",
		Protocol:               "",
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
type Config struct {
	Endpoint    string            `json:"endpoint" env:"OTEL_EXPORTER_OTLP_ENDPOINT,OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"`
	Protocol    string            `json:"protocol" env:"OTEL_EXPORTER_OTLP_PROTOCOL,OTEL_EXPORTER_OTLP_TRACES_PROTOCOL"`
	Timeout     string            `json:"timeout" env:"OTEL_EXPORTER_OTLP_TIMEOUT,OTEL_EXPORTER_OTLP_TRACES_TIMEOUT"`
	Headers     map[string]string `json:"otlp_headers" env:"OTEL_EXPORTER_OTLP_HEADERS"` // TODO: needs json marshaler hook to mask tokens
	Insecure    bool              `json:"insecure" env:"OTEL_EXPORTER_OTLP_INSECURE"`
	Blocking    bool              `json:"otlp_blocking" env:"OTEL_EXPORTER_OTLP_BLOCKING"`
	NoTlsVerify bool              `json:"no_tls_verify" env:"OTEL_CLI_NO_TLS_VERIFY"`

	ServiceName       string            `json:"service_name" env:"OTEL_CLI_SERVICE_NAME,OTEL_SERVICE_NAME"`
	SpanName          string            `json:"span_name" env:"OTEL_CLI_SPAN_NAME"`
	Kind              string            `json:"span_kind" env:"OTEL_CLI_TRACE_KIND"`
	Attributes        map[string]string `json:"span_attributes" env:"OTEL_CLI_ATTRIBUTES"`
	StatusCode        string            `json:"span_status_code" env:"OTEL_CLI_STATUS_CODE"`
	StatusDescription string            `json:"span_status_description" env:"OTEL_CLI_STATUS_DESCRIPTION"`

	TraceparentCarrierFile string `json:"traceparent_carrier_file" env:"OTEL_CLI_CARRIER_FILE"`
	TraceparentIgnoreEnv   bool   `json:"traceparent_ignore_env" env:"OTEL_CLI_IGNORE_ENV"`
	TraceparentPrint       bool   `json:"traceparent_print" env:"OTEL_CLI_PRINT_TRACEPARENT"`
	TraceparentPrintExport bool   `json:"traceparent_print_export" env:"OTEL_CLI_EXPORT_TRACEPARENT"`
	TraceparentRequired    bool   `json:"traceparent_required" env:"OTEL_CLI_TRACEPARENT_REQUIRED"`

	BackgroundParentPollMs int    `json:"background_parent_poll_ms" env:""`
	BackgroundSockdir      string `json:"background_socket_directory" env:""`
	BackgroundWait         bool   `json:"background_wait" env:""`

	SpanStartTime string `json:"span_start_time" env:""`
	SpanEndTime   string `json:"span_end_time" env:""`
	EventName     string `json:"event_name" env:""`
	EventTime     string `json:"event_time" env:""`

	CfgFile string `json:"config_file" env:"OTEL_CLI_CONFIG_FILE"`
	Verbose bool   `json:"verbose" env:"OTEL_CLI_VERBOSE"`
	Fail    bool   `json:"fail" env:"OTEL_CLI_FAIL"`
}

// LoadFile reads the file specified by -c/--config and overwrites the
// current config values with any found in the file.
func (c *Config) LoadFile() error {
	if config.CfgFile == "" {
		return nil
	}

	js, err := os.ReadFile(config.CfgFile)
	if err != nil {
		return errors.Wrapf(err, "failed to read file '%s'", c.CfgFile)
	}

	if err := json.Unmarshal(js, c); err != nil {
		return errors.Wrapf(err, "failed to parse json data in file '%s'", c.CfgFile)
	}

	return nil
}

// LoadEnv loads environment variables into the config, overwriting current
// values. Environment variable to config key mapping is tagged on the
// Config struct. Multiple names for envvars is supported, comma-separated.
// Takes a func(string)string that's usually os.Getenv, and is swappable to
// make testing easier.
func (c *Config) LoadEnv(getenv func(string) string) error {
	// loop over each field of the Config
	structType := reflect.TypeOf(c).Elem()
	cValue := reflect.ValueOf(c).Elem()
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		envVars := field.Tag.Get("env")
		if envVars == "" {
			continue
		}
		// a field can have multiple comma-delimiated env vars to look in
		for _, envVar := range strings.Split(envVars, ",") {
			// call the provided func(string)string provided to get the
			// envvar, usually os.Getenv but can be a fake for testing
			envVal := getenv(envVar)

			// prevent OTel SDK and child processes from reading config envvars
			os.Unsetenv(envVar)

			if envVal == "" {
				continue
			}

			// type switch and write the value into the struct
			target := cValue.Field(i)
			switch target.Interface().(type) {
			case string:
				target.SetString(envVal)
			case int:
				intVal, err := strconv.ParseInt(envVal, 10, 64)
				if err != nil {
					return errors.Wrapf(err, "could not parse %s value %q as an int", envVar, envVal)
				}
				target.SetInt(intVal)
			case bool:
				boolVal, err := strconv.ParseBool(envVal)
				if err != nil {
					return errors.Wrapf(err, "could not parse %s value %q as an bool", envVar, envVal)
				}
				target.SetBool(boolVal)
			case map[string]string:
				mapVal, err := parseCkvStringMap(envVal)
				if err != nil {
					return errors.Wrapf(err, "could not parse %s value %q as a map", envVar, envVal)
				}
				mapValVal := reflect.ValueOf(mapVal)
				target.Set(mapValVal)
			}
		}
	}

	return nil
}

// ToStringMap flattens the configuration into a stringmap that is easy to work
// with in tests especially with cmp.Diff. See test_main.go.
func (c Config) ToStringMap() map[string]string {
	return map[string]string{
		"endpoint":                    c.Endpoint,
		"protocol":                    c.Protocol,
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

// WithProtocol returns the config with protocol set to the provided value.
func (c Config) WithProtocol(with string) Config {
	c.Protocol = with
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

// WithFail returns the config with Fail set to the provided value.
func (c Config) WithFail(with bool) Config {
	c.Fail = with
	return c
}
