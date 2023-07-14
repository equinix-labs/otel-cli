package main_test

// This file implements data structures and data for functional testing of
// otel-cli.
//
// See: TESTING.md
//
// TODO: Results.SpanData could become a struct now

import (
	"regexp"
	"testing"
	"time"

	"github.com/equinix-labs/otel-cli/otlpclient"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

type serverProtocol int

const (
	grpcProtocol serverProtocol = iota
	httpProtocol
)

// CheckFunc is a function that gets called after the test is run to do
// custom checking of values.
type CheckFunc func(t *testing.T, fixture Fixture, results Results)

type FixtureConfig struct {
	CliArgs []string
	Env     map[string]string
	// timeout for how long to wait for the whole test in failure cases
	TestTimeoutMs int
	// when true this test will be excluded under go -test.short mode
	// TODO: maybe move this up to the suite?
	IsLongTest bool
	// either grpcProtocol or httpProtocol, defaults to grpc
	ServerProtocol serverProtocol
	// sets up the server with the test CA, requiring TLS
	ServerTLSEnabled bool
	// tells the server to require client certificate authentication
	ServerTLSAuthEnabled bool
	// for timeout tests we need to start the server to generate the endpoint
	// but do not want it to answer when otel-cli calls, this does that
	StopServerBeforeExec bool
	// run this fixture in the background, starting its server and otel-cli
	// instance, then let those block in the background and continue running
	// serial tests until it's "foreground" by a second fixtue with the same
	// description in the same file
	Background bool
	Foreground bool
}

// mostly mirrors otelcli.StatusOutput but we need more
type Results struct {
	// same as otelcli.StatusOutput but copied because embedding doesn't work for this
	Config      otlpclient.Config      `json:"config"`
	SpanData    map[string]string      `json:"span_data"`
	Env         map[string]string      `json:"env"`
	Diagnostics otlpclient.Diagnostics `json:"diagnostics"`
	// these are specific to tests...
	ServerMeta    map[string]string
	Headers       map[string]string // headers sent by the client
	ResourceSpans *tracepb.ResourceSpans
	CliOutput     string         // merged stdout and stderr
	CliOutputRe   *regexp.Regexp // regular expression to clean the output before comparison
	SpanCount     int            // number of spans received
	EventCount    int            // number of events received
	TimedOut      bool           // true when test timed out
	CommandFailed bool           // otel-cli failed / was killed
	Span          *tracepb.Span
	SpanEvents    []*tracepb.Span_Event
}

// Fixture represents a test fixture for otel-cli.
type Fixture struct {
	Name       string
	Config     FixtureConfig
	Endpoint   string
	TlsData    TlsSettings
	Expect     Results
	CheckFuncs []CheckFunc
}

// FixtureSuite is a list of Fixtures that run serially.
type FixtureSuite []Fixture

var suites = []FixtureSuite{
	// otel-cli should not do anything when it is not explicitly configured"
	{
		{
			Name: "nothing configured",
			Config: FixtureConfig{
				CliArgs: []string{"status"},
			},
			Expect: Results{
				Config: otlpclient.DefaultConfig(),
				Diagnostics: otlpclient.Diagnostics{
					IsRecording:     false,
					NumArgs:         1,
					ParsedTimeoutMs: 1000,
				},
			},
		},
	},
	// setting minimum envvars should result in a span being received
	{
		{
			Name: "minimum configuration (recording, grpc)",
			Config: FixtureConfig{
				ServerProtocol: grpcProtocol,
				CliArgs:        []string{"status", "--endpoint", "{{endpoint}}"},
				TestTimeoutMs:  1000,
			},
			Expect: Results{
				// otel-cli should NOT set insecure when it auto-detects localhost
				Config: otlpclient.DefaultConfig().
					WithEndpoint("{{endpoint}}").
					WithInsecure(false),
				ServerMeta: map[string]string{
					"proto": "grpc",
				},
				Diagnostics: otlpclient.Diagnostics{
					IsRecording:       true,
					NumArgs:           3,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					Endpoint:          "*",
					EndpointSource:    "*",
				},
				SpanCount: 1,
			},
		}, {
			Name: "minimum configuration (recording, http)",
			Config: FixtureConfig{
				ServerProtocol: httpProtocol,
				CliArgs:        []string{"status", "--endpoint", "http://{{endpoint}}"},
				TestTimeoutMs:  1000,
			},
			Expect: Results{
				// otel-cli should NOT set insecure when it auto-detects localhost
				Config: otlpclient.DefaultConfig().
					WithEndpoint("http://{{endpoint}}").
					WithInsecure(false),
				ServerMeta: map[string]string{
					"content-type": "application/x-protobuf",
					"host":         "{{endpoint}}",
					"method":       "POST",
					"proto":        "HTTP/1.1",
					"uri":          "/v1/traces",
				},
				Diagnostics: otlpclient.Diagnostics{
					IsRecording:       true,
					NumArgs:           3,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					Endpoint:          "*",
					EndpointSource:    "*",
				},
				SpanCount: 1,
			},
		},
	},
	// TLS connections
	{
		{
			Name: "minimum configuration (tls, no-verify, recording, grpc)",
			Config: FixtureConfig{
				ServerProtocol: grpcProtocol,
				CliArgs: []string{
					"status",
					"--endpoint", "https://{{endpoint}}",
					"--protocol", "grpc",
					// TODO: switch to --tls-no-verify before 1.0, for now keep testing it
					"--verbose", "--fail", "--no-tls-verify",
				},
				TestTimeoutMs:    1000,
				ServerTLSEnabled: true,
			},
			Expect: Results{
				Config: otlpclient.DefaultConfig().
					WithEndpoint("https://{{endpoint}}").
					WithProtocol("grpc").
					WithVerbose(true).
					WithTlsNoVerify(true),
				Diagnostics: otlpclient.Diagnostics{
					IsRecording:        true,
					NumArgs:            8,
					DetectedLocalhost:  true,
					InsecureSkipVerify: true,
					ParsedTimeoutMs:    1000,
					Endpoint:           "*",
					EndpointSource:     "*",
				},
				SpanCount: 1,
			},
		},
		{
			Name: "minimum configuration (tls, no-verify, recording, https)",
			Config: FixtureConfig{
				ServerProtocol:   httpProtocol,
				CliArgs:          []string{"status", "--endpoint", "https://{{endpoint}}", "--tls-no-verify"},
				TestTimeoutMs:    2000,
				ServerTLSEnabled: true,
			},
			Expect: Results{
				// otel-cli should NOT set insecure when it auto-detects localhost
				Config: otlpclient.DefaultConfig().
					WithTlsNoVerify(true).
					WithEndpoint("https://{{endpoint}}"),
				Diagnostics: otlpclient.Diagnostics{
					IsRecording:       true,
					NumArgs:           4,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					Endpoint:          "*",
					EndpointSource:    "*",
				},
				SpanCount: 1,
			},
		},
		{
			Name: "minimum configuration (tls, client cert auth, recording, grpc)",
			Config: FixtureConfig{
				ServerProtocol: grpcProtocol,
				CliArgs: []string{
					"status",
					"--endpoint", "https://{{endpoint}}",
					"--protocol", "grpc",
					"--verbose", "--fail",
					"--tls-ca-cert", "{{tls_ca_cert}}",
					"--tls-client-cert", "{{tls_client_cert}}",
					"--tls-client-key", "{{tls_client_key}}",
				},
				TestTimeoutMs:        1000,
				ServerTLSEnabled:     true,
				ServerTLSAuthEnabled: true,
			},
			Expect: Results{
				Config: otlpclient.DefaultConfig().
					WithEndpoint("https://{{endpoint}}").
					WithProtocol("grpc").
					WithTlsCACert("{{tls_ca_cert}}").
					WithTlsClientKey("{{tls_client_key}}").
					WithTlsClientCert("{{tls_client_cert}}").
					WithVerbose(true),
				Diagnostics: otlpclient.Diagnostics{
					IsRecording:        true,
					NumArgs:            13,
					DetectedLocalhost:  true,
					InsecureSkipVerify: true,
					ParsedTimeoutMs:    1000,
					Endpoint:           "*",
					EndpointSource:     "*",
				},
				SpanCount: 1,
			},
		},
		{
			Name: "minimum configuration (tls, client cert auth, recording, https)",
			Config: FixtureConfig{
				ServerProtocol: httpProtocol,
				CliArgs: []string{
					"status",
					"--endpoint", "https://{{endpoint}}",
					"--verbose", "--fail",
					"--tls-ca-cert", "{{tls_ca_cert}}",
					"--tls-client-cert", "{{tls_client_cert}}",
					"--tls-client-key", "{{tls_client_key}}",
				},
				TestTimeoutMs:        2000,
				ServerTLSEnabled:     true,
				ServerTLSAuthEnabled: true,
			},
			Expect: Results{
				Config: otlpclient.DefaultConfig().
					WithEndpoint("https://{{endpoint}}").
					WithTlsCACert("{{tls_ca_cert}}").
					WithTlsClientKey("{{tls_client_key}}").
					WithTlsClientCert("{{tls_client_cert}}").
					WithVerbose(true),
				Diagnostics: otlpclient.Diagnostics{
					IsRecording:       true,
					NumArgs:           11,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					Endpoint:          "*",
					EndpointSource:    "*",
				},
				SpanCount: 1,
			},
		},
	},
	// ensure things fail when they're supposed to fail
	{
		// otel is configured but there is no server listening so it should time out silently
		{
			Name: "timeout with no server",
			Config: FixtureConfig{
				CliArgs: []string{"span", "--timeout", "1s"},
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}",
				},
				// this needs to be less than the timeout in CliArgs
				TestTimeoutMs:        500,
				IsLongTest:           true, // can be skipped with `go test -short`
				StopServerBeforeExec: true, // there will be no server listening
			},
			Expect: Results{
				Config: otlpclient.DefaultConfig(),
				// we want and expect a timeout and failure
				TimedOut:      true,
				CommandFailed: true,
			},
		},
		{
			Name: "syntax errors in environment variables cause the command to fail",
			Config: FixtureConfig{
				CliArgs: []string{"span", "--fail", "--verbose"},
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}",
					"OTEL_CLI_VERBOSE":            "lmao", // invalid input
				},
			},
			Expect: Results{
				Config:        otlpclient.DefaultConfig(),
				CommandFailed: true,
				// strips the date off the log line before comparing to expectation
				CliOutputRe: regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} `),
				CliOutput: "Error while loading environment variables: could not parse OTEL_CLI_VERBOSE value " +
					"\"lmao\" as an bool: strconv.ParseBool: parsing \"lmao\": invalid syntax\n",
			},
		},
		{
			Name: "https:// should fail when TLS is not available",
			Config: FixtureConfig{
				ServerProtocol: httpProtocol,
				CliArgs:        []string{"status", "--endpoint", "https://{{endpoint}}"},
				TestTimeoutMs:  1000,
			},
			Expect: Results{
				Config: otlpclient.DefaultConfig().
					WithEndpoint("https://{{endpoint}}"),
				Diagnostics: otlpclient.Diagnostics{
					IsRecording:       true,
					NumArgs:           3,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					Endpoint:          "*",
					EndpointSource:    "*",
					Error:             `Post "https://{{endpoint}}/v1/traces": http: server gave HTTP response to HTTPS client`,
				},
				SpanCount: 0,
			},
		},
	},
	// regression tests
	{
		{
			// The span end time was missing when #175 merged, which showed up
			// as 0ms spans. CheckFuncs was added to make this possible. This
			// test runs sleep for 10ms and checks the duration of the span is
			// at least 10ms.
			Name: "#189 otel-cli exec sets span start time earlier than end time",
			Config: FixtureConfig{
				// note: relies on system sleep command supporting floats
				// note: 10ms is hardcoded in a few places for this test and commentary
				CliArgs: []string{"exec", "--endpoint", "{{endpoint}}", "sleep", "0.01"},
			},
			Expect: Results{
				SpanCount: 1,
				Config:    otlpclient.DefaultConfig().WithEndpoint("grpc://{{endpoint}}"),
			},
			CheckFuncs: []CheckFunc{
				func(t *testing.T, f Fixture, r Results) {
					//elapsed := r.Span.End.Sub(r.Span.Start)
					elapsed := time.Duration((r.Span.EndTimeUnixNano - r.Span.StartTimeUnixNano) * uint64(time.Nanosecond))
					if elapsed.Milliseconds() < 10 {
						t.Errorf("elapsed test time not long enough. Expected 10ms, got %d ms", elapsed.Milliseconds())
					}
				},
			},
		},
		{
			Name: "#181 OTEL_ envvars should persist through to otel-cli exec",
			Config: FixtureConfig{
				CliArgs: []string{"status"},
				Env: map[string]string{
					"OTEL_FAKE_VARIABLE":             "fake value",
					"OTEL_EXPORTER_OTLP_ENDPOINT":    "{{endpoint}}",
					"OTEL_EXPORTER_OTLP_CERTIFICATE": "{{tls_ca_cert}}",
					"X_WHATEVER":                     "whatever",
				},
			},
			Expect: Results{
				SpanCount: 1,
				Config:    otlpclient.DefaultConfig().WithEndpoint("{{endpoint}}").WithTlsCACert("{{tls_ca_cert}}"),
				Env: map[string]string{
					"OTEL_FAKE_VARIABLE":             "fake value",
					"OTEL_EXPORTER_OTLP_ENDPOINT":    "{{endpoint}}",
					"OTEL_EXPORTER_OTLP_CERTIFICATE": "{{tls_ca_cert}}",
					"X_WHATEVER":                     "whatever",
				},
				Diagnostics: otlpclient.Diagnostics{
					IsRecording:       true,
					DetectedLocalhost: true,
					NumArgs:           1,
					ParsedTimeoutMs:   1000,
					Endpoint:          "*",
					EndpointSource:    "*",
				},
			},
		},
		{
			Name: "#200 custom trace path in general endpoint gets signal path appended",
			Config: FixtureConfig{
				CliArgs:        []string{"status", "--endpoint", "http://{{endpoint}}/mycollector"},
				ServerProtocol: httpProtocol,
			},
			Expect: Results{
				SpanCount: 1,
				Config:    otlpclient.DefaultConfig().WithEndpoint("http://{{endpoint}}/mycollector"),
				Diagnostics: otlpclient.Diagnostics{
					IsRecording:       true,
					DetectedLocalhost: true,
					NumArgs:           3,
					ParsedTimeoutMs:   1000,
					// spec says /v1/traces should get appended to any general endpoint URL
					Endpoint:       "http://{{endpoint}}/mycollector/v1/traces",
					EndpointSource: "general",
				},
			},
		},
		{
			Name: "#200 custom trace path on signal endpoint does not get modified",
			Config: FixtureConfig{
				CliArgs:        []string{"status", "--traces-endpoint", "http://{{endpoint}}/mycollector/x/1"},
				ServerProtocol: httpProtocol,
			},
			Expect: Results{
				SpanCount: 1,
				Config:    otlpclient.DefaultConfig().WithTracesEndpoint("http://{{endpoint}}/mycollector/x/1"),
				Diagnostics: otlpclient.Diagnostics{
					IsRecording:       true,
					DetectedLocalhost: true,
					NumArgs:           3,
					ParsedTimeoutMs:   1000,
					Endpoint:          "http://{{endpoint}}/mycollector/x/1",
					EndpointSource:    "signal",
				},
			},
		},
	},
	// otel-cli span with no OTLP config should do and print nothing
	{
		{
			Name: "otel-cli span (unconfigured, non-recording)",
			Config: FixtureConfig{
				CliArgs: []string{"span", "--service", "main_test.go", "--name", "test-span-123", "--kind", "server"},
			},
			Expect: Results{Config: otlpclient.DefaultConfig()},
		},
	},
	// config file
	{
		{
			Name: "load a json config file",
			Config: FixtureConfig{
				CliArgs: []string{"status", "--config", "example-config.json"},
				// this will take priority over the config
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}",
				},
				TestTimeoutMs: 1000,
			},
			Expect: Results{
				SpanCount: 1,
				Diagnostics: otlpclient.Diagnostics{
					IsRecording:       true,
					NumArgs:           3,
					ParsedTimeoutMs:   1000,
					Endpoint:          "*",
					EndpointSource:    "*",
					DetectedLocalhost: true,
					Error:             "could not open file '/tmp/traceparent.txt' for read: open /tmp/traceparent.txt: no such file or directory",
				},
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}",
				},
				Config: otlpclient.DefaultConfig().
					WithEndpoint("{{endpoint}}"). // tells the test framework to ignore/overwrite
					WithTimeout("1s").
					WithHeaders(map[string]string{"header1": "header1-value"}).
					WithInsecure(true).
					WithBlocking(false).
					WithTlsNoVerify(true).
					WithTlsCACert("/dev/null").
					WithTlsClientKey("/dev/null").
					WithTlsClientCert("/dev/null").
					WithServiceName("configured_in_config_file").
					WithSpanName("config_file_span").
					WithKind("server").
					WithAttributes(map[string]string{"attr1": "value1"}).
					WithStatusCode("0").
					WithStatusDescription("status description").
					WithTraceparentCarrierFile("/tmp/traceparent.txt").
					WithTraceparentIgnoreEnv(true).
					WithTraceparentPrint(true).
					WithTraceparentPrintExport(true).
					WithTraceparentRequired(false).
					WithBackgroundParentPollMs(100).
					WithBackgroundSockdir("/tmp").
					WithBackgroundWait(true).
					WithSpanEndTime("now").
					WithSpanEndTime("now").
					WithEventName("config_file_event").
					WithEventTime("now").
					WithCfgFile("example-config.json").
					WithVerbose(true).
					WithFail(true),
			},
		},
	},
	// otel-cli with minimal config span sends a span that looks right
	{
		{
			Name: "otel-cli span (recording)",
			Config: FixtureConfig{
				CliArgs: []string{"span", "--service", "main_test.go", "--name", "test-span-123", "--kind", "server"},
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}",
				},
				TestTimeoutMs: 1000,
			},
			Expect: Results{
				Config:    otlpclient.DefaultConfig(),
				SpanCount: 1,
			},
		},
		// OTEL_RESOURCE_ATTRIBUTES and OTEL_CLI_SERVICE_NAME should get merged into
		// the span resource attributes
		{
			Name: "otel-cli span with envvar service name and attributes (recording)",
			Config: FixtureConfig{
				CliArgs: []string{"span", "--name", "test-span-service-name-and-attrs", "--kind", "server"},
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}",
					"OTEL_CLI_SERVICE_NAME":       "test-service-abc123",
					"OTEL_CLI_ATTRIBUTES":         "cafe=deadbeef,abc=123",
					"OTEL_RESOURCE_ATTRIBUTES":    "foo.bar=baz",
				},
				TestTimeoutMs: 1000,
			},
			Expect: Results{
				Config: otlpclient.DefaultConfig(),
				SpanData: map[string]string{
					"span_id":            "*",
					"trace_id":           "*",
					"attributes":         "abc=123,cafe=deadbeef",
					"service_attributes": "foo.bar=baz,service.name=test-service-abc123",
				},
				SpanCount: 1,
			},
		},
		// OTEL_SERVICE_NAME
		{
			Name: "otel-cli span with envvar service name (recording)",
			Config: FixtureConfig{
				CliArgs: []string{"span"},
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}",
					"OTEL_SERVICE_NAME":           "test-service-123abc",
				},
				TestTimeoutMs: 1000,
			},
			Expect: Results{
				Config: otlpclient.DefaultConfig(),
				SpanData: map[string]string{
					"service_attributes": "service.name=test-service-123abc",
				},
				SpanCount: 1,
			},
		},
	},
	// otel-cli span --print-tp actually prints
	{
		{
			Name: "otel-cli span --print-tp (non-recording)",
			Config: FixtureConfig{
				CliArgs: []string{"span", "--tp-print"},
				Env:     map[string]string{"TRACEPARENT": "00-f6c109f48195b451c4def6ab32f47b61-a5d2a35f2483004e-01"},
			},
			Expect: Results{
				Config: otlpclient.DefaultConfig(),
				CliOutput: "" + // empty so the text below can indent and line up
					"# trace id: f6c109f48195b451c4def6ab32f47b61\n" +
					"#  span id: a5d2a35f2483004e\n" +
					"TRACEPARENT=00-f6c109f48195b451c4def6ab32f47b61-a5d2a35f2483004e-01\n",
			},
		},
	},
	// otel-cli span --print-tp propagates traceparent even when not recording
	{
		{
			Name: "otel-cli span --tp-print --tp-export (non-recording)",
			Config: FixtureConfig{
				CliArgs: []string{"span", "--tp-print", "--tp-export"},
				Env: map[string]string{
					"TRACEPARENT": "00-f6c109f48195b451c4def6ab32f47b61-a5d2a35f2483004e-00",
				},
			},
			Expect: Results{
				Config: otlpclient.DefaultConfig(),
				CliOutput: "" +
					"# trace id: f6c109f48195b451c4def6ab32f47b61\n" +
					"#  span id: a5d2a35f2483004e\n" +
					"export TRACEPARENT=00-f6c109f48195b451c4def6ab32f47b61-a5d2a35f2483004e-00\n",
			},
		},
	},
	// otel-cli span background, non-recording, this uses the suite functionality
	// and background tasks, which are a little clunky but get the job done
	{
		{
			Name: "otel-cli span background (nonrecording)",
			Config: FixtureConfig{
				CliArgs:       []string{"span", "background", "--timeout", "1s", "--sockdir", "."},
				TestTimeoutMs: 2000,
				Background:    true,  // sorta like & in shell
				Foreground:    false, // must be true later, like `fg` in shell
			},
			Expect: Results{Config: otlpclient.DefaultConfig()},
		},
		{
			Name: "otel-cli span event",
			Config: FixtureConfig{
				CliArgs: []string{"span", "event", "--name", "an event happened", "--attrs", "ima=now,mondai=problem", "--sockdir", "."},
			},
			Expect: Results{Config: otlpclient.DefaultConfig()},
		},
		{
			Name: "otel-cli span end",
			Config: FixtureConfig{
				CliArgs: []string{"span", "end", "--sockdir", "."},
			},
			Expect: Results{Config: otlpclient.DefaultConfig()},
		},
		{
			// Name on foreground *must* match the backgrounded job
			// TODO: ^^ this isn't great, find a better way
			Name: "otel-cli span background (nonrecording)",
			Config: FixtureConfig{
				Foreground: true, // bring it back (fg) and finish up
			},
			Expect: Results{Config: otlpclient.DefaultConfig()},
		},
	},
	// otel-cli span background, in recording mode
	{
		{
			Name: "otel-cli span background (recording)",
			Config: FixtureConfig{
				CliArgs:       []string{"span", "background", "--timeout", "1s", "--sockdir", ".", "--attrs", "abc=def"},
				Env:           map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}"},
				TestTimeoutMs: 2000,
				Background:    true,
				Foreground:    false,
			},
			Expect: Results{
				Config: otlpclient.DefaultConfig(),
				SpanData: map[string]string{
					"span_id":    "*",
					"trace_id":   "*",
					"attributes": `abc=def`, // weird format because of limitation in OTLP server
				},
				SpanCount:  1,
				EventCount: 1,
			},
			// this validates options sent to otel-cli span end
			CheckFuncs: []CheckFunc{
				func(t *testing.T, f Fixture, r Results) {
					if r.Span.Status.GetCode() != 2 {
						t.Errorf("expected 2 for span status code, but got %d", r.Span.Status.GetCode())
					}
					if r.Span.Status.GetMessage() != "I can't do that Dave." {
						t.Errorf("got wrong string for status description: %q", r.Span.Status.GetMessage())
					}
				},
			},
		},
		{
			Name: "otel-cli span event",
			Config: FixtureConfig{
				CliArgs: []string{"span", "event", "--name", "an event happened", "--attrs", "ima=now,mondai=problem", "--sockdir", "."},
			},
			Expect: Results{Config: otlpclient.DefaultConfig()},
		},
		{
			Name: "otel-cli span end",
			Config: FixtureConfig{
				CliArgs: []string{
					"span", "end",
					"--sockdir", ".",
					// these are validated by checkfuncs defined above ^^
					"--status-code", "error",
					"--status-description", "I can't do that Dave.",
				},
			},
			Expect: Results{Config: otlpclient.DefaultConfig()},
		},
		{
			Name: "otel-cli span background (recording)",
			Config: FixtureConfig{
				Foreground: true, // fg
			},
			Expect: Results{Config: otlpclient.DefaultConfig()},
		},
	},
	// otel-cli exec runs echo
	{
		{
			Name: "otel-cli span exec echo",
			Config: FixtureConfig{
				// intentionally calling a command with no args bc it's a special case in exec.go
				CliArgs: []string{"exec", "--service", "main_test.go", "--name", "test-span-123", "--kind", "server", "echo"},
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}",
					"TRACEPARENT":                 "00-edededededededededededededed9000-edededededededed-01",
				},
			},
			Expect: Results{
				Config: otlpclient.DefaultConfig(),
				SpanData: map[string]string{
					"span_id":  "*",
					"trace_id": "edededededededededededededed9000",
				},
				CliOutput: "\n",
				SpanCount: 1,
			},
		},
	},
	// otel-cli exec runs otel-cli exec
	{
		{
			Name: "otel-cli span exec (nested)",
			Config: FixtureConfig{
				CliArgs: []string{
					"exec", "--name", "outer", "--endpoint", "{{endpoint}}", "--fail", "--verbose", "--",
					"./otel-cli", "exec", "--name", "inner", "--endpoint", "{{endpoint}}", "--tp-required", "--fail", "--verbose", "echo", "hello world"},
			},
			Expect: Results{
				Config:    otlpclient.DefaultConfig(),
				CliOutput: "hello world\n",
				SpanCount: 2,
			},
		},
	},
	// validate OTEL_EXPORTER_OTLP_PROTOCOL / --protocol
	{
		// --protocol
		{
			Name: "--protocol grpc",
			Config: FixtureConfig{
				ServerProtocol: grpcProtocol,
				CliArgs:        []string{"status", "--endpoint", "{{endpoint}}", "--protocol", "grpc"},
				TestTimeoutMs:  1000,
			},
			Expect: Results{
				Config: otlpclient.DefaultConfig().WithEndpoint("{{endpoint}}").WithProtocol("grpc"),
				ServerMeta: map[string]string{
					"proto": "grpc",
				},
				Diagnostics: otlpclient.Diagnostics{
					IsRecording:       true,
					NumArgs:           5,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					Endpoint:          "*",
					EndpointSource:    "*",
				},
				SpanCount: 1,
			},
		},
		{
			Name: "--protocol http/protobuf",
			Config: FixtureConfig{
				ServerProtocol: httpProtocol,
				CliArgs:        []string{"status", "--endpoint", "http://{{endpoint}}", "--protocol", "http/protobuf"},
				TestTimeoutMs:  1000,
			},
			Expect: Results{
				Config: otlpclient.DefaultConfig().WithEndpoint("http://{{endpoint}}").WithProtocol("http/protobuf"),
				ServerMeta: map[string]string{
					"content-type": "application/x-protobuf",
					"host":         "{{endpoint}}",
					"method":       "POST",
					"proto":        "HTTP/1.1",
					"uri":          "/v1/traces",
				},
				Diagnostics: otlpclient.Diagnostics{
					IsRecording:       true,
					NumArgs:           5,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					Endpoint:          "*",
					EndpointSource:    "*",
				},
				SpanCount: 1,
			},
		},
		{
			Name: "protocol: bad config",
			Config: FixtureConfig{
				CliArgs:       []string{"status", "--endpoint", "{{endpoint}}", "--protocol", "xxx", "--verbose", "--fail"},
				TestTimeoutMs: 1000,
			},
			Expect: Results{
				CommandFailed: true,
				CliOutputRe:   regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} `),
				CliOutput:     "invalid protocol setting \"xxx\"\n",
				Config:        otlpclient.DefaultConfig().WithEndpoint("{{endpoint}}"),
				Diagnostics: otlpclient.Diagnostics{
					IsRecording:       false,
					NumArgs:           7,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					Endpoint:          "*",
					EndpointSource:    "*",
				},
				SpanCount: 0,
			},
		},
		// OTEL_EXPORTER_OTLP_PROTOCOL
		{
			Name: "OTEL_EXPORTER_OTLP_PROTOCOL grpc",
			Config: FixtureConfig{
				ServerProtocol: grpcProtocol,
				// validate protocol can be set to grpc with an http endpoint
				CliArgs:       []string{"status", "--endpoint", "http://{{endpoint}}"},
				TestTimeoutMs: 1000,
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_PROTOCOL": "grpc",
				},
			},
			Expect: Results{
				Config: otlpclient.DefaultConfig().WithEndpoint("http://{{endpoint}}").WithProtocol("grpc"),
				ServerMeta: map[string]string{
					"proto": "grpc",
				},
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_PROTOCOL": "grpc",
				},
				Diagnostics: otlpclient.Diagnostics{
					IsRecording:       true,
					NumArgs:           3,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					Endpoint:          "*",
					EndpointSource:    "*",
				},
				SpanCount: 1,
			},
		},
		{
			Name: "OTEL_EXPORTER_OTLP_PROTOCOL http/protobuf",
			Config: FixtureConfig{
				ServerProtocol: httpProtocol,
				CliArgs:        []string{"status", "--endpoint", "http://{{endpoint}}"},
				TestTimeoutMs:  1000,
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf",
				},
			},
			Expect: Results{
				Config: otlpclient.DefaultConfig().WithEndpoint("http://{{endpoint}}").WithProtocol("http/protobuf"),
				ServerMeta: map[string]string{
					"content-type": "application/x-protobuf",
					"host":         "{{endpoint}}",
					"method":       "POST",
					"proto":        "HTTP/1.1",
					"uri":          "/v1/traces",
				},
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf",
				},
				Diagnostics: otlpclient.Diagnostics{
					IsRecording:       true,
					NumArgs:           3,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					Endpoint:          "*",
					EndpointSource:    "*",
				},
				SpanCount: 1,
			},
		},
		{
			Name: "OTEL_EXPORTER_OTLP_PROTOCOL: bad config",
			Config: FixtureConfig{
				CliArgs:       []string{"status", "--endpoint", "http://{{endpoint}}", "--fail", "--verbose"},
				TestTimeoutMs: 1000,
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_PROTOCOL": "roflcopter",
				},
			},
			Expect: Results{
				CommandFailed: true,
				CliOutputRe:   regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} `),
				CliOutput:     "invalid protocol setting \"roflcopter\"\n",
				Config:        otlpclient.DefaultConfig().WithEndpoint("http://{{endpoint}}"),
				Diagnostics: otlpclient.Diagnostics{
					IsRecording:       false,
					NumArgs:           3,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					Endpoint:          "*",
					EndpointSource:    "*",
					Error:             "invalid protocol setting \"roflcopter\"\n",
				},
				SpanCount: 0,
			},
		},
	},
	// --force-trace-id, --force-span-id and --force-parent-span-id allow setting/forcing custom trace, span and parent span ids
	{
		{
			Name: "forced trace, span and parent span ids",
			Config: FixtureConfig{
				CliArgs: []string{
					"status",
					"--endpoint", "{{endpoint}}",
					"--force-trace-id", "00112233445566778899aabbccddeeff",
					"--force-span-id", "beefcafefacedead",
					"--force-parent-span-id", "p4r3ntb33fc4f3d3",
				},
			},
			Expect: Results{
				Config: otlpclient.DefaultConfig().WithEndpoint("{{endpoint}}"),
				SpanData: map[string]string{
					"trace_id":       "00112233445566778899aabbccddeeff",
					"span_id":        "beefcafefacedead",
					"parent_span_id": "e4e3eeb33fc4f3d3",
				},
				SpanCount: 1,
				Diagnostics: otlpclient.Diagnostics{
					NumArgs:           7,
					IsRecording:       true,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					Endpoint:          "*",
					EndpointSource:    "*",
				},
			},
		},
	},
	// full-system test --otlp-headers makes it to grpc/http servers
	{
		{
			Name: "#231 gRPC headers for authentication",
			Config: FixtureConfig{
				CliArgs: []string{
					"status",
					"--endpoint", "{{endpoint}}",
					"--protocol", "grpc",
					"--otlp-headers", "x-otel-cli-otlpserver-token=abcdefgabcdefg",
				},
				ServerProtocol: grpcProtocol,
			},
			Expect: Results{
				SpanCount: 1,
				Config: otlpclient.DefaultConfig().
					WithEndpoint("{{endpoint}}").
					WithProtocol("grpc").
					WithHeaders(map[string]string{
						"x-otel-cli-otlpserver-token": "abcdefgabcdefg",
					}),
				Headers: map[string]string{
					":authority":                  "{{endpoint}}\n",
					"content-type":                "application/grpc\n",
					"user-agent":                  "*",
					"x-otel-cli-otlpserver-token": "abcdefgabcdefg\n",
				},
				Diagnostics: otlpclient.Diagnostics{
					IsRecording:       true,
					DetectedLocalhost: true,
					NumArgs:           7,
					ParsedTimeoutMs:   1000,
					Endpoint:          "grpc://{{endpoint}}",
					EndpointSource:    "general",
				},
			},
		},
		{
			Name: "#231 http headers for authentication",
			Config: FixtureConfig{
				CliArgs: []string{
					"status",
					"--endpoint", "http://{{endpoint}}",
					"--protocol", "http/protobuf",
					"--otlp-headers", "x-otel-cli-otlpserver-token=abcdefgabcdefg",
				},
				ServerProtocol: httpProtocol,
			},
			Expect: Results{
				SpanCount: 1,
				Config: otlpclient.DefaultConfig().
					WithEndpoint("http://{{endpoint}}").
					WithProtocol("http/protobuf").
					WithHeaders(map[string]string{
						"x-otel-cli-otlpserver-token": "abcdefgabcdefg",
					}),
				Headers: map[string]string{
					"Content-Type":                "application/x-protobuf",
					"Accept-Encoding":             "gzip",
					"User-Agent":                  "Go-http-client/1.1",
					"Content-Length":              "232",
					"X-Otel-Cli-Otlpserver-Token": "abcdefgabcdefg",
				},
				Diagnostics: otlpclient.Diagnostics{
					IsRecording:       true,
					DetectedLocalhost: true,
					NumArgs:           7,
					ParsedTimeoutMs:   1000,
					Endpoint:          "http://{{endpoint}}/v1/traces",
					EndpointSource:    "general",
				},
			},
		},
	},
}
