package main_test

// Data structures and data for functional testing of otel-cli.

// TODO: Results.SpanData could become a struct now
// TODO: add instructions for adding more tests

import "github.com/equinix-labs/otel-cli/otelcli"

type FixtureConfig struct {
	CliArgs []string
	Env     map[string]string
	// timeout for how long to wait for the whole test in failure cases
	TestTimeoutMs int
	// when true this test will be excluded under go -test.short mode
	// TODO: maybe move this up to the suite?
	IsLongTest bool
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
	CliOutput     string // merged stdout and stderr
	Spans         int    // number of spans received
	Events        int    // number of events received
	TimedOut      bool   // true when test timed out
	CommandFailed bool   // otel-cli failed / was killed
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
			Name: "minimum configuration (recording)",
			Config: FixtureConfig{
				CliArgs:       []string{"status"},
				Env:           map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}"},
				TestTimeoutMs: 1000,
			},
			Expect: Results{
				// otel-cli should NOT set insecure when it auto-detects localhost
				Config: otelcli.DefaultConfig().
					WithEndpoint("{{endpoint}}").
					WithInsecure(false),
				SpanData: map[string]string{
					"span_id":  "*",
					"trace_id": "*",
				},
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}",
				},
				Diagnostics: otelcli.Diagnostics{
					IsRecording:       true,
					NumArgs:           1,
					DetectedLocalhost: true,
					ParsedTimeoutMs:   1000,
					OtelError:         "",
				},
				Spans: 1,
			},
		},
	},
	// otel is configured but there is no server listening so it should time out silently
	{
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
					"attributes": `abc=string_value:"def"`, // weird format because of limitation in OTLP server
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
				CliArgs: []string{"exec", "--service", "main_test.go", "--name", "test-span-123", "--kind", "server", "./otel-cli", "exec", "--tp-ignore-env", "echo hello world $TRACEPARENT"},
				Env: map[string]string{
					"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}",
				},
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
}
