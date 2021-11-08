package otelcli

import (
	"strconv"
	"strings"
)

// global diagnostics handle, written to from all over otel-cli
var diagnostics Diagnostics

// Diagnostics is a place to put things that are useful for testing and
// diagnosing issues with otel-cli. The only user-facing feature that should be
// using these is otel-cli status.
type Diagnostics struct {
	CliArgs           []string `json:"cli_args"`
	IsRecording       bool     `json:"is_recording"`
	ConfigFileLoaded  bool     `json:"config_file_loaded"`
	NumArgs           int      `json:"number_of_args"`
	DetectedLocalhost bool     `json:"detected_localhost"`
	ParsedTimeoutMs   int64    `json:"parsed_timeout_ms"`
	OtelError         string   `json:"otel_error"`
	ExecExitCode      int      `json:"exec_exit_code"`
}

// Handle complies with the otel error handler interface to capture errors
// both for diagnostics and to make sure the error output goes through softLog
// so it doesn't pollute output of caller scripts.
// hack: ignores the Diagnostics assigned to it and writes to the global
func (Diagnostics) Handle(err error) {
	diagnostics.OtelError = err.Error() // write to the global

	if config.Fail {
		softFail("OpenTelemetry error: %s", err)
	} else {
		softLog("OpenTelemetry error: %s", err)
	}
}

// ToMap returns the Diagnostics struct as a string map for testing.
func (d *Diagnostics) ToStringMap() map[string]string {
	return map[string]string{
		"cli_args":           strings.Join(d.CliArgs, " "),
		"is_recording":       strconv.FormatBool(d.IsRecording),
		"config_file_loaded": strconv.FormatBool(d.ConfigFileLoaded),
		"number_of_args":     strconv.Itoa(d.NumArgs),
		"detected_localhost": strconv.FormatBool(d.DetectedLocalhost),
		"parsed_timeout_ms":  strconv.FormatInt(d.ParsedTimeoutMs, 10),
		"otel_error":         d.OtelError,
	}
}

// GetExitCode() is a helper for Cobra to retrieve the exit code, mainly
// used by exec to make otel-cli return the child program's exit code.
func GetExitCode() int {
	return diagnostics.ExecExitCode
}
