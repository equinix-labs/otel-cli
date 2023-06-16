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
	CliArgs            []string `json:"cli_args"`
	IsRecording        bool     `json:"is_recording"`
	ConfigFileLoaded   bool     `json:"config_file_loaded"`
	NumArgs            int      `json:"number_of_args"`
	DetectedLocalhost  bool     `json:"detected_localhost"`
	InsecureSkipVerify bool     `json:"insecure_skip_verify"`
	ParsedTimeoutMs    int64    `json:"parsed_timeout_ms"`
	Endpoint           string   `json:"endpoint"` // the computed endpoint, not the raw config val
	EndpointSource     string   `json:"endpoint_source"`
	Error              string   `json:"error"`
	ExecExitCode       int      `json:"exec_exit_code"`
	Retries            int      `json:"retries"`
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
		"endpoint":           d.Endpoint,
		"endpoint_source":    d.EndpointSource,
		"error":              d.Error,
	}
}

// SetError sets the diagnostics Error to the error's string if it's
// not nil and returns the same error so it can be inlined in return.
func (d *Diagnostics) SetError(err error) error {
	if err != nil {
		diagnostics.Error = err.Error()
	}
	return err
}

// GetExitCode() is a helper for Cobra to retrieve the exit code, mainly
// used by exec to make otel-cli return the child program's exit code.
func GetExitCode() int {
	return diagnostics.ExecExitCode
}
