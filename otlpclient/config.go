package otlpclient

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

var detectBrokenRFC3339PrefixRe *regexp.Regexp
var epochNanoTimeRE *regexp.Regexp

func init() {
	detectBrokenRFC3339PrefixRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} `)
	epochNanoTimeRE = regexp.MustCompile(`^\d+\.\d+$`)
}

// DefaultConfig returns a Config with all defaults set.
func DefaultConfig() Config {
	return Config{
		Endpoint:                     "",
		Protocol:                     "",
		Timeout:                      "1s",
		Headers:                      map[string]string{},
		Insecure:                     false,
		Blocking:                     false,
		TlsNoVerify:                  false,
		TlsCACert:                    "",
		TlsClientKey:                 "",
		TlsClientCert:                "",
		ServiceName:                  "otel-cli",
		SpanName:                     "todo-generate-default-span-names",
		Kind:                         "client",
		ForceTraceId:                 "",
		ForceSpanId:                  "",
		Attributes:                   map[string]string{},
		TraceparentCarrierFile:       "",
		TraceparentIgnoreEnv:         false,
		TraceparentPrint:             false,
		TraceparentPrintExport:       false,
		TraceparentRequired:          false,
		BackgroundParentPollMs:       10,
		BackgroundSockdir:            "",
		BackgroundWait:               false,
		BackgroundSkipParentPidCheck: false,
		StatusCanaryCount:            1,
		StatusCanaryInterval:         "",
		SpanStartTime:                "now",
		SpanEndTime:                  "now",
		EventName:                    "todo-generate-default-event-names",
		EventTime:                    "now",
		CfgFile:                      "",
		Verbose:                      false,
		Fail:                         false,
		StatusCode:                   "unset",
		StatusDescription:            "",
	}
}

// Config stores the runtime configuration for otel-cli.
// Data structure is public so that it can serialize to json easily.
type Config struct {
	Endpoint       string            `json:"endpoint" env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	TracesEndpoint string            `json:"traces_endpoint" env:"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"`
	Protocol       string            `json:"protocol" env:"OTEL_EXPORTER_OTLP_PROTOCOL,OTEL_EXPORTER_OTLP_TRACES_PROTOCOL"`
	Timeout        string            `json:"timeout" env:"OTEL_EXPORTER_OTLP_TIMEOUT,OTEL_EXPORTER_OTLP_TRACES_TIMEOUT"`
	Headers        map[string]string `json:"otlp_headers" env:"OTEL_EXPORTER_OTLP_HEADERS"` // TODO: needs json marshaler hook to mask tokens
	Insecure       bool              `json:"insecure" env:"OTEL_EXPORTER_OTLP_INSECURE"`
	Blocking       bool              `json:"otlp_blocking" env:"OTEL_EXPORTER_OTLP_BLOCKING"`

	TlsCACert     string `json:"tls_ca_cert" env:"OTEL_EXPORTER_OTLP_CERTIFICATE,OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE"`
	TlsClientKey  string `json:"tls_client_key" env:"OTEL_EXPORTER_OTLP_CLIENT_KEY,OTEL_EXPORTER_OTLP_TRACES_CLIENT_KEY"`
	TlsClientCert string `json:"tls_client_cert" env:"OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE,OTEL_EXPORTER_OTLP_TRACES_CLIENT_CERTIFICATE"`
	// OTEL_CLI_NO_TLS_VERIFY is deprecated and will be removed for 1.0
	TlsNoVerify bool `json:"tls_no_verify" env:"OTEL_CLI_TLS_NO_VERIFY,OTEL_CLI_NO_TLS_VERIFY"`

	ServiceName       string            `json:"service_name" env:"OTEL_CLI_SERVICE_NAME,OTEL_SERVICE_NAME"`
	SpanName          string            `json:"span_name" env:"OTEL_CLI_SPAN_NAME"`
	Kind              string            `json:"span_kind" env:"OTEL_CLI_TRACE_KIND"`
	Attributes        map[string]string `json:"span_attributes" env:"OTEL_CLI_ATTRIBUTES"`
	StatusCode        string            `json:"span_status_code" env:"OTEL_CLI_STATUS_CODE"`
	StatusDescription string            `json:"span_status_description" env:"OTEL_CLI_STATUS_DESCRIPTION"`
	ForceSpanId       string            `json:"force_span_id" env:"OTEL_CLI_FORCE_SPAN_ID"`
	ForceTraceId      string            `json:"force_trace_id" env:"OTEL_CLI_FORCE_TRACE_ID"`

	TraceparentCarrierFile string `json:"traceparent_carrier_file" env:"OTEL_CLI_CARRIER_FILE"`
	TraceparentIgnoreEnv   bool   `json:"traceparent_ignore_env" env:"OTEL_CLI_IGNORE_ENV"`
	TraceparentPrint       bool   `json:"traceparent_print" env:"OTEL_CLI_PRINT_TRACEPARENT"`
	TraceparentPrintExport bool   `json:"traceparent_print_export" env:"OTEL_CLI_EXPORT_TRACEPARENT"`
	TraceparentRequired    bool   `json:"traceparent_required" env:"OTEL_CLI_TRACEPARENT_REQUIRED"`

	BackgroundParentPollMs       int    `json:"background_parent_poll_ms" env:""`
	BackgroundSockdir            string `json:"background_socket_directory" env:""`
	BackgroundWait               bool   `json:"background_wait" env:""`
	BackgroundSkipParentPidCheck bool   `json:"background_skip_parent_pid_check"`

	StatusCanaryCount    int    `json:"status_canary_count"`
	StatusCanaryInterval string `json:"status_canary_interval"`

	SpanStartTime string `json:"span_start_time" env:""`
	SpanEndTime   string `json:"span_end_time" env:""`
	EventName     string `json:"event_name" env:""`
	EventTime     string `json:"event_time" env:""`

	CfgFile string `json:"config_file" env:"OTEL_CLI_CONFIG_FILE"`
	Verbose bool   `json:"verbose" env:"OTEL_CLI_VERBOSE"`
	Fail    bool   `json:"fail" env:"OTEL_CLI_FAIL"`

	// not exported, used to get data from cobra to otlpclient internals
	StartupTime time.Time `json:"-"`
	Version     string    `json:"-"`
}

// LoadFile reads the file specified by -c/--config and overwrites the
// current config values with any found in the file.
func (c *Config) LoadFile() error {
	if c.CfgFile == "" {
		return nil
	}

	js, err := os.ReadFile(c.CfgFile)
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
		"tls_no_verify":               strconv.FormatBool(c.TlsNoVerify),
		"tls_ca_cert":                 c.TlsCACert,
		"tls_client_key":              c.TlsClientKey,
		"tls_client_cert":             c.TlsClientCert,
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
		"background_skip_pid_check":   strconv.FormatBool(c.BackgroundSkipParentPidCheck),
		"span_start_time":             c.SpanStartTime,
		"span_end_time":               c.SpanEndTime,
		"event_name":                  c.EventName,
		"event_time":                  c.EventTime,
		"config_file":                 c.CfgFile,
		"verbose":                     strconv.FormatBool(c.Verbose),
	}
}

// IsRecording returns true if an endpoint is set and otel-cli expects to send real
// spans. Returns false if unconfigured and going to run inert.
func (c Config) IsRecording() bool {
	if c.Endpoint == "" && c.TracesEndpoint == "" {
		Diag.IsRecording = false
		return false
	}

	Diag.IsRecording = true
	return true
}

// ParseCliTimeout parses the --timeout string value to a time.Duration.
func (c Config) ParseCliTimeout() time.Duration {
	out, err := parseDuration(c.Timeout)
	Diag.ParsedTimeoutMs = out.Milliseconds()
	c.SoftFailIfErr(err)
	return out
}

// ParseStatusCanaryInterval parses the --canary-interval string value to a time.Duration.
func (c Config) ParseStatusCanaryInterval() time.Duration {
	out, err := parseDuration(c.StatusCanaryInterval)
	c.SoftFailIfErr(err)
	return out
}

// parseDuration parses a string duration into a time.Duration.
// When no duration letter is provided (e.g. ms, s, m, h), seconds are assumed.
// It logs an error and returns time.Duration(0) if the string is empty or unparseable.
func parseDuration(d string) (time.Duration, error) {
	var out time.Duration
	if d == "" {
		out = time.Duration(0)
	} else if parsed, err := time.ParseDuration(d); err == nil {
		out = parsed
	} else if secs, serr := strconv.ParseInt(d, 10, 0); serr == nil {
		out = time.Second * time.Duration(secs)
	} else {
		return time.Duration(0), fmt.Errorf("unable to parse duration string %q: %s", d, err)
	}

	return out, nil
}

// SoftLog only calls through to log if otel-cli was run with the --verbose flag.
// TODO: does it make any sense to support %w? probably yes, can clean up some
// diagnostics.Error touch points.
func (c Config) SoftLog(format string, a ...interface{}) {
	if !c.Verbose {
		return
	}
	log.Printf(format, a...)
}

// SoftLogIfErr calls SoftLog only if err != nil.
// Written as an interim step to pushing errors up the stack instead of calling
// SoftLog/SoftFail directly in methods that don't need a config handle.
func (c Config) SoftLogIfErr(err error) {
	if err != nil {
		c.SoftLog(err.Error())
	}
}

// SoftFail calls through to softLog (which logs only if otel-cli was run with the --verbose
// flag), then immediately exits - with status -1 by default, or 1 if --fail was
// set (a la `curl --fail`)
func (c Config) SoftFail(format string, a ...interface{}) {
	c.SoftLog(format, a...)

	if c.Fail {
		os.Exit(1)
	} else {
		os.Exit(0)
	}
}

// SoftFailIfErr calls SoftFail only if err != nil.
// Written as an interim step to pushing errors up the stack instead of calling
// SoftLog/SoftFail directly in methods that don't need a config handle.
func (c Config) SoftFailIfErr(err error) {
	if err != nil {
		c.SoftFail(err.Error())
	}
}

// flattenStringMap takes a string map and returns it flattened into a string with
// keys sorted lexically so it should be mostly consistent enough for comparisons
// and printing. Output is k=v,k=v style like attributes input.
func flattenStringMap(mp map[string]string, emptyValue string) string {
	if len(mp) == 0 {
		return emptyValue
	}

	var out string
	keys := make([]string, len(mp)) // for sorting
	var i int
	for k := range mp {
		keys[i] = k
		i++
	}
	sort.Strings(keys)

	for i, k := range keys {
		out = out + k + "=" + mp[k]
		if i == len(keys)-1 {
			break
		}
		out = out + ","
	}

	return out
}

// parseCkvStringMap parses key=value,foo=bar formatted strings as a line of CSV
// and returns it as a string map.
func parseCkvStringMap(in string) (map[string]string, error) {
	r := csv.NewReader(strings.NewReader(in))
	pairs, err := r.Read()
	if err != nil {
		return map[string]string{}, err
	}

	out := make(map[string]string)
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if parts[0] != "" && parts[1] != "" {
			out[parts[0]] = parts[1]
		} else {
			return map[string]string{}, fmt.Errorf("kv pair %s must be in key=value format", pair)
		}
	}

	return out, nil
}

// ParsedSpanStartTime returns config.SpanStartTime as time.Time.
func (c Config) ParsedSpanStartTime() time.Time {
	t, err := c.parseTime(c.SpanStartTime, "start")
	c.SoftFailIfErr(err)
	return t
}

// ParsedSpanEndTime returns config.SpanEndTime as time.Time.
func (c Config) ParsedSpanEndTime() time.Time {
	t, err := c.parseTime(c.SpanEndTime, "end")
	c.SoftFailIfErr(err)
	return t
}

// ParsedEventTime returns config.EventTime as time.Time.
func (c Config) ParsedEventTime() time.Time {
	t, err := c.parseTime(c.EventTime, "event")
	c.SoftFailIfErr(err)
	return t
}

// parseTime tries to parse Unix epoch, then RFC3339, both with/without nanoseconds
func (c Config) parseTime(ts, which string) (time.Time, error) {
	var uterr, utnerr, utnnerr, rerr, rnerr error

	if ts == "now" {
		return time.Now(), nil
	}

	// Unix epoch time
	if i, uterr := strconv.ParseInt(ts, 10, 64); uterr == nil {
		return time.Unix(i, 0), nil
	}

	// date --rfc-3339 returns an invalid format for Go because it has a
	// space instead of 'T' between date and time
	if detectBrokenRFC3339PrefixRe.MatchString(ts) {
		ts = strings.Replace(ts, " ", "T", 1)
	}

	// Unix epoch time with nanoseconds
	if epochNanoTimeRE.MatchString(ts) {
		parts := strings.Split(ts, ".")
		if len(parts) == 2 {
			secs, utnerr := strconv.ParseInt(parts[0], 10, 64)
			nsecs, utnnerr := strconv.ParseInt(parts[1], 10, 64)
			if utnerr == nil && utnnerr == nil && secs > 0 {
				return time.Unix(secs, nsecs), nil
			}
		}
	}

	// try RFC3339 then again with nanos
	t, rerr := time.Parse(time.RFC3339, ts)
	if rerr != nil {
		t, rnerr := time.Parse(time.RFC3339Nano, ts)
		if rnerr == nil {
			return t, nil
		}
	} else {
		return t, nil
	}

	// none of the formats worked, print whatever errors are remaining
	if uterr != nil {
		return time.Time{}, fmt.Errorf("could not parse span %s time %q as Unix Epoch: %s", which, ts, uterr)
	}
	if utnerr != nil || utnnerr != nil {
		return time.Time{}, fmt.Errorf("could not parse span %s time %q as Unix Epoch.Nano: %s | %s", which, ts, utnerr, utnnerr)
	}
	if rerr != nil {
		return time.Time{}, fmt.Errorf("could not parse span %s time %q as RFC3339: %s", which, ts, rerr)
	}
	if rnerr != nil {
		return time.Time{}, fmt.Errorf("could not parse span %s time %q as RFC3339Nano: %s", which, ts, rnerr)
	}

	return time.Time{}, fmt.Errorf("could not parse span %s time %q as any supported format", which, ts)
}

// WithEndpoint returns the config with Endpoint set to the provided value.
func (c Config) WithEndpoint(with string) Config {
	c.Endpoint = with
	return c
}

// WithTracesEndpoint returns the config with TracesEndpoint set to the provided value.
func (c Config) WithTracesEndpoint(with string) Config {
	c.TracesEndpoint = with
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

// WithTlsNoVerify returns the config with NoTlsVerify set to the provided value.
func (c Config) WithTlsNoVerify(with bool) Config {
	c.TlsNoVerify = with
	return c
}

// WithTlsCACert returns the config with TlsCACert set to the provided value.
func (c Config) WithTlsCACert(with string) Config {
	c.TlsCACert = with
	return c
}

// WithTlsClientKey returns the config with NoTlsClientKey set to the provided value.
func (c Config) WithTlsClientKey(with string) Config {
	c.TlsClientKey = with
	return c
}

// WithTlsClientCert returns the config with NoTlsClientCert set to the provided value.
func (c Config) WithTlsClientCert(with string) Config {
	c.TlsClientCert = with
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

// WithBackgroundSkipParentPidCheck returns the config with BackgroundSkipParentPidCheck set to the provided value.
func (c Config) WithBackgroundSkipParentPidCheck(with bool) Config {
	c.BackgroundSkipParentPidCheck = with
	return c
}

// WithStatusCanaryCount returns the config with StatusCanaryCount set to the provided value.
func (c Config) WithStatusCanaryCount(with int) Config {
	c.StatusCanaryCount = with
	return c
}

// WithStatusCanaryInterval returns the config with StatusCanaryInterval set to the provided value.
func (c Config) WithStatusCanaryInterval(with string) Config {
	c.StatusCanaryInterval = with
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
