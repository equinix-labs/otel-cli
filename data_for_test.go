package main_test

// Data structures and data for functional testing of otel-cli.

// TODO: strip defaults from the data structures (might mean dumping them again or a bit of manual work...)
// TODO: add instructions for adding more tests

import "github.com/equinix-labs/otel-cli/cmd"

type FixtureConfig struct {
	CliArgs []string `json:"cli_args"`
	Env     map[string]string
	// timeout for how long to wait for the whole test in failure cases
	TestTimeoutMs int `json:"test_timeout_ms"`
	// when true this test will be excluded under go -test.short mode
	// TODO: maybe move this up to the suite?
	IsLongTest bool `json:"is_long_test"`
	// for timeout tests we need to start the server to generate the endpoint
	// but do not want it to answer when otel-cli calls, this does that
	StopServerBeforeExec bool `json:"stop_server_before_exec"`
	// run this fixture in the background, starting its server and otel-cli
	// instance, then let those block in the background and continue running
	// serial tests until it's "foreground" by a second fixtue with the same
	// description in the same file
	Background bool `json:"background"`
	Foreground bool `json:"foreground"`
}

// mostly mirrors cmd.StatusOutput but we need more
type Results struct {
	// the same datastructure used to generate otel-cli status output
	cmd.StatusOutput
	CliOutput     string `json:"output"`         // merged stdout and stderr
	Spans         int    `json:"spans"`          // number of spans received
	Events        int    `json:"events"`         // number of events received
	TimedOut      bool   `json:"timed_out"`      // true when test timed out
	CommandFailed bool   `json:"command_failed"` // otel-cli failed / was killed
}

// Fixture represents a test fixture for otel-cli.
type Fixture struct {
	Description string        `json:"description"`
	Filename    string        `json:"-"` // populated at runtime
	Config      FixtureConfig `json:"config"`
	Expect      Results       `json:"expect"`
}

// FixtureSuite is a list of Fixtures that run serially.
type FixtureSuite []Fixture

var suites = []FixtureSuite{
	{
		{
			Description: "otel-cli should not do anything when it is not explicitly configured",
			Filename:    "00-unconfigured.json",
			Config: FixtureConfig{
				CliArgs:              []string{"status"},
				Env:                  map[string]string{},
				TestTimeoutMs:        0,
				IsLongTest:           false,
				StopServerBeforeExec: false,
				Background:           false,
				Foreground:           false,
			},
			Expect: Results{
				StatusOutput: cmd.StatusOutput{
					Config: cmd.Config{
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
					},
					SpanData: map[string]string{},
					Env:      map[string]string{},
					Diagnostics: cmd.Diagnostics{
						CliArgs:           nil,
						IsRecording:       false,
						ConfigFileLoaded:  false,
						NumArgs:           1,
						DetectedLocalhost: false,
						ParsedTimeoutMs:   0,
						OtelError:         "",
						ExecExitCode:      0,
					},
				},
				CliOutput:     "",
				Spans:         0,
				Events:        0,
				TimedOut:      false,
				CommandFailed: false,
			},
		},
	},
	{
		{
			Description: "setting minimum envvars should result in a span being received",
			Filename:    "20-basic-configuration.json",
			Config: FixtureConfig{
				CliArgs:              []string{"status"},
				Env:                  map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}"},
				TestTimeoutMs:        1000,
				IsLongTest:           false,
				StopServerBeforeExec: false,
				Background:           false,
				Foreground:           false,
			},
			Expect: Results{
				StatusOutput: cmd.StatusOutput{
					Config: cmd.Config{
						Endpoint:               "{{endpoint}}",
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
					},
					SpanData: map[string]string{"span_id": "*", "trace_id": "*"},
					Env:      map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}"},
					Diagnostics: cmd.Diagnostics{
						CliArgs:           nil,
						IsRecording:       true,
						ConfigFileLoaded:  false,
						NumArgs:           1,
						DetectedLocalhost: true,
						ParsedTimeoutMs:   1000,
						OtelError:         "",
						ExecExitCode:      0,
					},
				},
				CliOutput:     "",
				Spans:         1,
				Events:        0,
				TimedOut:      false,
				CommandFailed: false,
			},
		},
	},
	{
		{
			Description: "otel is configured but there is no server listening so it should time out silently",
			Filename:    "21-basic-timeout.json",
			Config: FixtureConfig{
				CliArgs:              []string{"span", "--timeout", "1s"},
				Env:                  map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}"},
				TestTimeoutMs:        500,
				IsLongTest:           true,
				StopServerBeforeExec: true,
				Background:           false,
				Foreground:           false,
			},
			Expect: Results{
				StatusOutput:  cmd.StatusOutput{},
				CliOutput:     "",
				Spans:         0,
				Events:        0,
				TimedOut:      true,
				CommandFailed: true,
			},
		},
	},
	{
		{
			Description: "otel-cli span with no OTLP config should do and print nothing",
			Filename:    "50-span-nonrecording.json",
			Config: FixtureConfig{
				CliArgs:              []string{"span", "--service", "main_test.go", "--name", "test-span-123", "--kind", "server"},
				Env:                  map[string]string{},
				TestTimeoutMs:        0,
				IsLongTest:           false,
				StopServerBeforeExec: false,
				Background:           false,
				Foreground:           false,
			},
			Expect: Results{},
		},
	},
	{
		{
			Description: "otel-cli span sends a span",
			Filename:    "51-span-recording.json",
			Config: FixtureConfig{
				CliArgs:              []string{"span", "--service", "main_test.go", "--name", "test-span-123", "--kind", "server"},
				Env:                  map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}"},
				TestTimeoutMs:        1000,
				IsLongTest:           false,
				StopServerBeforeExec: false,
				Background:           false,
				Foreground:           false,
			},
			Expect: Results{
				StatusOutput: cmd.StatusOutput{
					Config:      cmd.Config{},
					SpanData:    map[string]string{"is_sampled": "true", "span_id": "*", "trace_id": "*"},
					Env:         map[string]string{},
					Diagnostics: cmd.Diagnostics{},
				},
				CliOutput:     "",
				Spans:         1,
				Events:        0,
				TimedOut:      false,
				CommandFailed: false,
			},
		},
	},
	{
		{
			Description: "otel-cli span --print-tp actually prints",
			Filename:    "52-span--print-tp.json",
			Config: FixtureConfig{
				CliArgs:              []string{"span", "--tp-print"},
				Env:                  map[string]string{"TRACEPARENT": "00-f6c109f48195b451c4def6ab32f47b61-a5d2a35f2483004e-01"},
				TestTimeoutMs:        0,
				IsLongTest:           false,
				StopServerBeforeExec: false,
				Background:           false,
				Foreground:           false,
			},
			Expect: Results{
				StatusOutput:  cmd.StatusOutput{},
				CliOutput:     "# trace id: 00000000000000000000000000000000\n#  span id: 0000000000000000\nTRACEPARENT=00-f6c109f48195b451c4def6ab32f47b61-a5d2a35f2483004e-01\n",
				Spans:         0,
				Events:        0,
				TimedOut:      false,
				CommandFailed: false,
			},
		},
	},
	{
		{
			Description: "otel-cli span --print-tp actually prints",
			Filename:    "53-span--tp-print--tp-export.json",
			Config: FixtureConfig{
				CliArgs:              []string{"span", "--tp-print", "--tp-export"},
				Env:                  map[string]string{"TRACEPARENT": "00-f6c109f48195b451c4def6ab32f47b61-a5d2a35f2483004e-01"},
				TestTimeoutMs:        0,
				IsLongTest:           false,
				StopServerBeforeExec: false,
				Background:           false,
				Foreground:           false,
			},
			Expect: Results{
				StatusOutput:  cmd.StatusOutput{},
				CliOutput:     "# trace id: 00000000000000000000000000000000\n#  span id: 0000000000000000\nexport TRACEPARENT=00-f6c109f48195b451c4def6ab32f47b61-a5d2a35f2483004e-01\n",
				Spans:         0,
				Events:        0,
				TimedOut:      false,
				CommandFailed: false,
			},
		},
	},
	{
		{
			Description: "otel-cli span background, non-recording",
			Filename:    "80-span-background-nonrecording.json",
			Config: FixtureConfig{
				CliArgs:              []string{"span", "background", "--timeout", "1s", "--sockdir", "."},
				Env:                  map[string]string{},
				TestTimeoutMs:        2000,
				IsLongTest:           false,
				StopServerBeforeExec: false,
				Background:           true,
				Foreground:           false,
			},
			Expect: Results{},
		},
		{
			Description: "otel-cli span event",
			Filename:    "80-span-background-nonrecording.json",
			Config: FixtureConfig{
				CliArgs:              []string{"span", "event", "--name", "an event happened", "--attrs", "ima=now,mondai=problem", "--sockdir", "."},
				Env:                  map[string]string{},
				TestTimeoutMs:        0,
				IsLongTest:           false,
				StopServerBeforeExec: false,
				Background:           false,
				Foreground:           false,
			},
			Expect: Results{},
		},
		{
			Description: "otel-cli span end",
			Filename:    "80-span-background-nonrecording.json",
			Config: FixtureConfig{
				CliArgs:              []string{"span", "end", "--sockdir", "."},
				Env:                  map[string]string{},
				TestTimeoutMs:        0,
				IsLongTest:           false,
				StopServerBeforeExec: false,
				Background:           false,
				Foreground:           false,
			},
			Expect: Results{},
		},
		{
			Description: "otel-cli span background, non-recording",
			Filename:    "80-span-background-nonrecording.json",
			Config: FixtureConfig{
				CliArgs:              []string{},
				Env:                  map[string]string{},
				TestTimeoutMs:        0,
				IsLongTest:           false,
				StopServerBeforeExec: false,
				Background:           false,
				Foreground:           true,
			},
			Expect: Results{},
		},
	},
	{
		{
			Description: "otel-cli span background",
			Filename:    "81-span-background.json",
			Config: FixtureConfig{
				CliArgs:              []string{"span", "background", "--timeout", "1s", "--sockdir", "."},
				Env:                  map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}"},
				TestTimeoutMs:        2000,
				IsLongTest:           false,
				StopServerBeforeExec: false,
				Background:           true,
				Foreground:           false,
			},
			Expect: Results{
				StatusOutput: cmd.StatusOutput{
					Config:      cmd.Config{},
					SpanData:    map[string]string{"span_id": "*", "trace_id": "*"},
					Env:         map[string]string{},
					Diagnostics: cmd.Diagnostics{},
				},
				CliOutput:     "",
				Spans:         1,
				Events:        1,
				TimedOut:      false,
				CommandFailed: false,
			},
		},
		{
			Description: "otel-cli span event",
			Filename:    "81-span-background.json",
			Config: FixtureConfig{
				CliArgs:              []string{"span", "event", "--name", "an event happened", "--attrs", "ima=now,mondai=problem", "--sockdir", "."},
				Env:                  map[string]string{},
				TestTimeoutMs:        0,
				IsLongTest:           false,
				StopServerBeforeExec: false,
				Background:           false,
				Foreground:           false,
			},
			Expect: Results{},
		},
		{
			Description: "otel-cli span end",
			Filename:    "81-span-background.json",
			Config: FixtureConfig{
				CliArgs:              []string{"span", "end", "--sockdir", "."},
				Env:                  map[string]string{},
				TestTimeoutMs:        0,
				IsLongTest:           false,
				StopServerBeforeExec: false,
				Background:           false,
				Foreground:           false,
			},
			Expect: Results{},
		},
		{
			Description: "otel-cli span background",
			Filename:    "81-span-background.json",
			Config: FixtureConfig{
				CliArgs:              nil,
				Env:                  map[string]string{},
				TestTimeoutMs:        0,
				IsLongTest:           false,
				StopServerBeforeExec: false,
				Background:           false,
				Foreground:           true,
			},
			Expect: Results{},
		},
	},
	{
		{
			Description: "otel-cli exec runs echo",
			Filename:    "90-span-exec-basic.json",
			Config: FixtureConfig{
				CliArgs:              []string{"exec", "--service", "main_test.go", "--name", "test-span-123", "--kind", "server", "echo hello world"},
				Env:                  map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}", "TRACEPARENT": "00-edededededededededededededed9000-edededededededed-01"},
				TestTimeoutMs:        0,
				IsLongTest:           false,
				StopServerBeforeExec: false,
				Background:           false,
				Foreground:           false,
			},
			Expect: Results{
				StatusOutput: cmd.StatusOutput{
					Config:      cmd.Config{},
					SpanData:    map[string]string{"is_sampled": "true", "span_id": "*", "trace_id": "edededededededededededededed9000"},
					Env:         map[string]string{},
					Diagnostics: cmd.Diagnostics{},
				},
				CliOutput:     "hello world\n",
				Spans:         1,
				Events:        0,
				TimedOut:      false,
				CommandFailed: false,
			},
		},
	},
	{
		{
			Description: "otel-cli exec runs otel-cli exec",
			Filename:    "91-span-exec-nested.json",
			Config: FixtureConfig{
				CliArgs:              []string{"exec", "--service", "main_test.go", "--name", "test-span-123", "--kind", "server", "./otel-cli", "exec", "--tp-ignore-env", "echo hello world $TRACEPARENT"},
				Env:                  map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "{{endpoint}}"},
				TestTimeoutMs:        0,
				IsLongTest:           false,
				StopServerBeforeExec: false,
				Background:           false,
				Foreground:           false,
			},
			Expect: Results{
				StatusOutput: cmd.StatusOutput{
					Config:      cmd.Config{},
					SpanData:    map[string]string{"is_sampled": "true", "span_id": "*", "trace_id": "*"},
					Env:         map[string]string{},
					Diagnostics: cmd.Diagnostics{},
				},
				CliOutput:     "hello world\n",
				Spans:         2,
				Events:        0,
				TimedOut:      false,
				CommandFailed: false,
			},
		},
	},
}
