package main_test

// Data structures and data for functional testing of otel-cli.

// TODO: Results.SpanData could become a struct now
// TODO: add instructions for adding more tests

import (
	"regexp"

	"github.com/equinix-labs/otel-cli/otelcli"
)

type serverProtocol int

const (
	grpcProtocol serverProtocol = iota
	httpProtocol
)

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
	Config      otelcli.Config      `json:"config"`
	SpanData    map[string]string   `json:"span_data"`
	Env         map[string]string   `json:"env"`
	Diagnostics otelcli.Diagnostics `json:"diagnostics"`
	// these are specific to tests...
	CliOutput     string         // merged stdout and stderr
	CliOutputRe   *regexp.Regexp // regular expression to clean the output before comparison
	Spans         int            // number of spans received
	Events        int            // number of events received
	TimedOut      bool           // true when test timed out
	CommandFailed bool           // otel-cli failed / was killed
}

// Fixture represents a test fixture for otel-cli.
type Fixture struct {
	Name   string
	Config FixtureConfig
	Expect Results
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
				Config: otelcli.DefaultConfig(),
				Diagnostics: otelcli.Diagnostics{
					IsRecording: false,
					NumArgs:     1,
					OtelError:   "",
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
				Config: otelcli.DefaultConfig().
					WithEndpoint("{{endpoint}}").
					WithInsecure(false),
				SpanData: map[string]string{
					"span_id":     "*",
					"trace_id":    "*",
					"server_meta": "proto=grpc",
				},
				Diagnostics: otelcli.Diagnostics{
					IsRecording:       true,
					NumArgs:           3,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					OtelError:         "",
				},
				Spans: 1,
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
				Config: otelcli.DefaultConfig().
					WithEndpoint("http://{{endpoint}}").
					WithInsecure(false),
				SpanData: map[string]string{
					"span_id":     "*",
					"trace_id":    "*",
					"server_meta": "host={{endpoint}},method=POST,proto=HTTP/1.1,uri=/v1/traces",
				},
				Diagnostics: otelcli.Diagnostics{
					IsRecording:       true,
					NumArgs:           3,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					OtelError:         "",
				},
				Spans: 1,
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
				Config: otelcli.DefaultConfig(),
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
				Config:        otelcli.DefaultConfig(),
				CommandFailed: true,
				// strips the date off the log line before comparing to expectation
				CliOutputRe: regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} `),
				CliOutput: "Error while loading environment variables: could not parse OTEL_CLI_VERBOSE value " +
					"\"lmao\" as an bool: strconv.ParseBool: parsing \"lmao\": invalid syntax\n",
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
			Expect: Results{Config: otelcli.DefaultConfig()},
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
				Spans: 1,
				Diagnostics: otelcli.Diagnostics{
					IsRecording:     true,
					NumArgs:         3,
					ParsedTimeoutMs: 1000,
				},
				Config: otelcli.DefaultConfig().
					WithEndpoint("{{endpoint}}"). // tells the test framework to ignore/overwrite
					WithTimeout("1s").
					WithHeaders(map[string]string{"header1": "header1-value"}).
					WithInsecure(true).
					WithBlocking(false).
					WithNoTlsVerify(true).
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
					WithTraceparentRequired(true).
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
				Config: otelcli.DefaultConfig(),
				SpanData: map[string]string{
					"span_id":  "*",
					"trace_id": "*",
				},
				Spans: 1,
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
				Config: otelcli.DefaultConfig(),
				SpanData: map[string]string{
					"span_id":            "*",
					"trace_id":           "*",
					"attributes":         "abc=123,cafe=deadbeef",
					"service_attributes": "foo.bar=baz,service.name=test-service-abc123",
				},
				Spans: 1,
			},
		},
	},
	// otel-cli span --print-tp actually prints
	{
		{
			Name: "otel-cli span --print-tp",
			Config: FixtureConfig{
				CliArgs: []string{"span", "--tp-print"},
				Env:     map[string]string{"TRACEPARENT": "00-f6c109f48195b451c4def6ab32f47b61-a5d2a35f2483004e-01"},
			},
			Expect: Results{
				Config: otelcli.DefaultConfig(),
				CliOutput: "" + // empty so the text below can indent and line up
					"# trace id: 00000000000000000000000000000000\n" +
					"#  span id: 0000000000000000\n" +
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
					"TRACEPARENT": "00-f6c109f48195b451c4def6ab32f47b61-a5d2a35f2483004e-01",
				},
			},
			Expect: Results{
				Config: otelcli.DefaultConfig(),
				CliOutput: "" +
					"# trace id: 00000000000000000000000000000000\n" +
					"#  span id: 0000000000000000\n" +
					"export TRACEPARENT=00-f6c109f48195b451c4def6ab32f47b61-a5d2a35f2483004e-01\n",
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
			Expect: Results{Config: otelcli.DefaultConfig()},
		},
		{
			Name: "otel-cli span event",
			Config: FixtureConfig{
				CliArgs: []string{"span", "event", "--name", "an event happened", "--attrs", "ima=now,mondai=problem", "--sockdir", "."},
			},
			Expect: Results{Config: otelcli.DefaultConfig()},
		},
		{
			Name: "otel-cli span end",
			Config: FixtureConfig{
				CliArgs: []string{"span", "end", "--sockdir", "."},
			},
			Expect: Results{Config: otelcli.DefaultConfig()},
		},
		{
			// Name on foreground *must* match the backgrounded job
			// TODO: ^^ this isn't great, find a better way
			Name: "otel-cli span background (nonrecording)",
			Config: FixtureConfig{
				Foreground: true, // bring it back (fg) and finish up
			},
			Expect: Results{Config: otelcli.DefaultConfig()},
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
				Config: otelcli.DefaultConfig(),
				SpanData: map[string]string{
					"span_id":    "*",
					"trace_id":   "*",
					"attributes": `abc=def`, // weird format because of limitation in OTLP server
				},
				Spans:  1,
				Events: 1,
			},
		},
		{
			Name: "otel-cli span event",
			Config: FixtureConfig{
				CliArgs: []string{"span", "event", "--name", "an event happened", "--attrs", "ima=now,mondai=problem", "--sockdir", "."},
			},
			Expect: Results{Config: otelcli.DefaultConfig()},
		},
		{
			Name: "otel-cli span end",
			Config: FixtureConfig{
				CliArgs: []string{"span", "end", "--sockdir", "."},
			},
			Expect: Results{Config: otelcli.DefaultConfig()},
		},
		{
			Name: "otel-cli span background (recording)",
			Config: FixtureConfig{
				Foreground: true, // fg
			},
			Expect: Results{Config: otelcli.DefaultConfig()},
		},
	},
	// otel-cli exec runs echo
	{
		{
			Name: "otel-cli span exec echo",
			Config: FixtureConfig{
				CliArgs: []string{"exec", "--service", "main_test.go", "--name", "test-span-123", "--kind", "server", "echo hello world"},
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}",
					"TRACEPARENT":                 "00-edededededededededededededed9000-edededededededed-01",
				},
			},
			Expect: Results{
				Config: otelcli.DefaultConfig(),
				SpanData: map[string]string{
					"span_id":  "*",
					"trace_id": "edededededededededededededed9000",
				},
				CliOutput: "hello world\n",
				Spans:     1,
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
					"./otel-cli", "exec", "--name", "inner", "--endpoint", "{{endpoint}}", "--tp-required", "--fail", "--verbose", "echo hello world"},
			},
			Expect: Results{
				Config: otelcli.DefaultConfig(),
				SpanData: map[string]string{
					"span_id":  "*",
					"trace_id": "*",
				},
				CliOutput: "hello world\n",
				Spans:     2,
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
				Config: otelcli.DefaultConfig().WithEndpoint("{{endpoint}}").WithProtocol("grpc"),
				SpanData: map[string]string{
					"server_meta": "proto=grpc",
				},
				Diagnostics: otelcli.Diagnostics{
					IsRecording:       true,
					NumArgs:           5,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					OtelError:         "",
				},
				Spans: 1,
			},
		},
		{
			Name: "--protocol http/protobuf",
			Config: FixtureConfig{
				ServerProtocol: httpProtocol,
				CliArgs:        []string{"status", "--endpoint", "{{endpoint}}", "--protocol", "http/protobuf"},
				TestTimeoutMs:  1000,
			},
			Expect: Results{
				Config: otelcli.DefaultConfig().WithEndpoint("{{endpoint}}").WithProtocol("http/protobuf"),
				SpanData: map[string]string{
					"server_meta": "proto=http/protobuf",
				},
				Diagnostics: otelcli.Diagnostics{
					IsRecording:       true,
					NumArgs:           5,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					OtelError:         "",
				},
				Spans: 1,
			},
		},
		{
			Name: "--protocol http/json",
			Config: FixtureConfig{
				ServerProtocol: httpProtocol,
				CliArgs:        []string{"status", "--endpoint", "{{endpoint}}", "--protocol", "http/json"},
				TestTimeoutMs:  1000,
			},
			Expect: Results{
				Config: otelcli.DefaultConfig().WithEndpoint("{{endpoint}}").WithProtocol("http/json"),
				SpanData: map[string]string{
					"server_meta": "proto=http/json",
				},
				Diagnostics: otelcli.Diagnostics{
					IsRecording:       true,
					NumArgs:           5,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					OtelError:         "",
				},
				Spans: 1,
			},
		},
		{
			Name: "protocol: bad config",
			Config: FixtureConfig{
				CliArgs:       []string{"status", "--endpoint", "{{endpoint}}", "--protocol", "xxx"},
				TestTimeoutMs: 1000,
			},
			Expect: Results{
				CommandFailed: true,
				Config:        otelcli.DefaultConfig().WithEndpoint("{{endpoint}}"),
				Diagnostics: otelcli.Diagnostics{
					IsRecording:       false,
					NumArgs:           5,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					OtelError:         "TODO(@tobert): FILL THIS IN",
				},
				Spans: 0,
			},
		},
		// OTEL_EXPORTER_OTLP_PROTOCOL
		{
			Name: "OTEL_EXPORTER_OTLP_PROTOCOL grpc",
			Config: FixtureConfig{
				ServerProtocol: grpcProtocol,
				// validate protocol can be set to grpc with an http endpoint
				CliArgs:       []string{"status", "--endpoint", "http://{{endpoint}}", "--protocol", "grpc"},
				TestTimeoutMs: 1000,
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_PROTOCOL": "grpc",
				},
			},
			Expect: Results{
				Config: otelcli.DefaultConfig().WithEndpoint("{{endpoint}}").WithProtocol("grpc"),
				SpanData: map[string]string{
					"server_meta": "proto=grpc",
				},
				Diagnostics: otelcli.Diagnostics{
					IsRecording:       true,
					NumArgs:           3,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					OtelError:         "",
				},
				Spans: 1,
			},
		},
		{
			Name: "OTEL_EXPORTER_OTLP_PROTOCOL http/protobuf",
			Config: FixtureConfig{
				ServerProtocol: httpProtocol,
				CliArgs:        []string{"status", "--endpoint", "{{endpoint}}"},
				TestTimeoutMs:  1000,
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf",
				},
			},
			Expect: Results{
				Config: otelcli.DefaultConfig().WithEndpoint("{{endpoint}}").WithProtocol("http/protobuf"),
				SpanData: map[string]string{
					"server_meta": "proto=http/protobuf",
				},
				Diagnostics: otelcli.Diagnostics{
					IsRecording:       true,
					NumArgs:           3,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					OtelError:         "",
				},
				Spans: 1,
			},
		},
		{
			Name: "OTEL_EXPORTER_OTLP_PROTOCOL http/json",
			Config: FixtureConfig{
				ServerProtocol: httpProtocol,
				CliArgs:        []string{"status", "--endpoint", "{{endpoint}}"},
				TestTimeoutMs:  1000,
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_PROTOCOL": "http/json",
				},
			},
			Expect: Results{
				Config: otelcli.DefaultConfig().WithEndpoint("{{endpoint}}").WithProtocol("http/json"),
				SpanData: map[string]string{
					"server_meta": "proto=http/json",
				},
				Diagnostics: otelcli.Diagnostics{
					IsRecording:       true,
					NumArgs:           3,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					OtelError:         "",
				},
				Spans: 1,
			},
		},
		{
			Name: "OTEL_EXPORTER_OTLP_PROTOCOL: bad config",
			Config: FixtureConfig{
				CliArgs:       []string{"status", "--endpoint", "{{endpoint}}"},
				TestTimeoutMs: 1000,
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_PROTOCOL": "roflcopter",
				},
			},
			Expect: Results{
				CommandFailed: true,
				Config:        otelcli.DefaultConfig().WithEndpoint("{{endpoint}}"),
				Diagnostics: otelcli.Diagnostics{
					IsRecording:       false,
					NumArgs:           3,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					OtelError:         "TODO(@tobert): FILL THIS IN",
				},
				Spans: 0,
			},
		},
	},
}
