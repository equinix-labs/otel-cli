package main

// end-to-end tests for otel-cli using json test definitions in ./fixtures

import (
	"encoding/json"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/equinix-labs/otel-cli/cmd"
	"github.com/equinix-labs/otel-cli/otlpserver"
	"github.com/google/go-cmp/cmp"
)

// otel-cli will fail with "getent not found" if PATH is empty
// so set it to the bare minimum and always the same for cleanup
const minimumPath = `/bin`

type FixtureConfig struct {
	CliArgs []string `json:"cli_args"`
	Env     map[string]string
}

// Fixture represents a test fixture for otel-cli.
type Fixture struct {
	Description     string           `json:"description"`
	Filename        string           `json:"-"`
	Config          FixtureConfig    `json:"config"`
	Expect          cmd.StatusOutput `json:"expect"`
	SpansExpected   int              `json:"spans_expected"`
	ServerTimeoutMs int              `json:"server_timeout_ms"`
	ShouldTimeout   bool             `json:"should_timeout"` // otel connection stub->cli should fail
}

func TestMain(m *testing.M) {
	// wipe out this process's envvars right away to avoid pollution & leakage
	os.Clearenv()
	result := m.Run()
	os.Exit(result)
}

// TestOtelCli loads all the json fixtures and executes the tests.
func TestOtelCli(t *testing.T) {
	wd, _ := os.Getwd() // go tests execute in the *_test.go's directory
	fixtureDir := filepath.Join(wd, "fixtures")
	files, err := ioutil.ReadDir(fixtureDir)
	if err != nil {
		t.Fatalf("Failed to list fixture directory %q to detect json files.", fixtureDir)
	}

	fixtures := []Fixture{}
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".json") {
			fixture := Fixture{}
			// initialize with a default config before reading the json so when
			// we compare against the output from otel-cli the defaults don't cause noise
			// v.s. an empty Config{}
			fixture.Expect.Config = cmd.DefaultConfig()

			fp := filepath.Join(fixtureDir, file.Name())
			js, err := os.ReadFile(fp)
			if err != nil {
				t.Fatalf("Failed to read json fixture file %q: %s", file.Name(), err)
			}
			err = json.Unmarshal(js, &fixture)
			if err != nil {
				t.Fatalf("Failed to parse json fixture file %q: %s", file.Name(), err)
			}

			// make sure PATH hasn't been set, because doing that in fixtures is naughty
			if _, ok := fixture.Config.Env["PATH"]; ok {
				t.Fatalf("fixture in file %s is not allowed to modify or test envvar PATH", file.Name())
			}

			fixture.Filename = filepath.Base(file.Name()) // for error reporting
			fixtures = append(fixtures, fixture)
		}
	}

	t.Logf("Loaded %d fixtures.", len(fixtures))
	if len(fixtures) == 0 {
		t.Fatal("no test fixtures loaded!")
	}

	// run all the fixtures, check the results
	for _, fixture := range fixtures {
		endpoint, status, span, events := runOtelCli(t, fixture)
		checkData(t, fixture, endpoint, status, span, events)
	}
}

// checkData takes the data returned from otel-cli status and compares it to the
// fixture data and fails the tests if anything doesn't match.
func checkData(t *testing.T, fixture Fixture, endpoint string, status cmd.StatusOutput, span otlpserver.CliEvent, events otlpserver.CliEventList) {
	// check the env
	injectEndpoint(endpoint, fixture.Expect.Env)
	if diff := cmp.Diff(fixture.Expect.Env, status.Env); diff != "" {
		t.Errorf("env data did not match fixture in %q (-want +got):\n%s", fixture.Filename, diff)
	}

	// check diagnostics, use string maps so the diff output is easy to compare to json
	wantDiag := fixture.Expect.Diagnostics.ToStringMap()
	gotDiag := status.Diagnostics.ToStringMap()
	injectEndpoint(endpoint, wantDiag)
	if diff := cmp.Diff(wantDiag, gotDiag); diff != "" {
		t.Errorf("diagnostic data did not match fixture in %q (-want +got):\n%s", fixture.Filename, diff)
	}

	// check the configuration
	wantConf := fixture.Expect.Config.ToStringMap()
	gotConf := status.Config.ToStringMap()
	injectEndpoint(endpoint, wantConf)
	if diff := cmp.Diff(wantConf, gotConf); diff != "" {
		t.Errorf("config data did not match fixture in %q (-want +got):\n%s", fixture.Filename, diff)
	}

	// check the expected span data against what was received by the OTLP server
	gotSpan := span.ToStringMap()
	injectEndpoint(endpoint, gotSpan)
	// remove keys that aren't supported for comparison (for now)
	delete(gotSpan, "is_populated")
	delete(gotSpan, "library")
	delete(gotSpan, "attributes")   // TODO: this one is kinda important to add back, eventually
	wantSpan := map[string]string{} // to be passed to cmp.Diff
	for what, re := range map[string]*regexp.Regexp{
		"trace_id": regexp.MustCompile(`^[0-9a-fA-F]{32}$`),
		"span_id":  regexp.MustCompile(`^[0-9a-fA-F]{16}$`),
		"name":     regexp.MustCompile(`^\w+$`),
		"parent":   regexp.MustCompile(`^[0-9a-fA-F]{32}$`),
		"kind":     regexp.MustCompile(`^\w+$`), // TODO: can validate more here
		"start":    regexp.MustCompile(`^\d+$`),
		"end":      regexp.MustCompile(`^\d+$`),
	} {
		// ignore anything not asked for in the fixture by deleting it from the gotSpan
		if _, ok := fixture.Expect.SpanData[what]; !ok {
			delete(gotSpan, what)
		}

		if wantVal, ok := fixture.Expect.SpanData[what]; ok {
			wantSpan[what] = wantVal // make a straight copy to make cmp.Diff happy
			if gotVal, ok := gotSpan[what]; ok {
				// * means if the above RE returns cleanly then pass the test
				if wantVal == "*" {
					if re.MatchString(gotVal) {
						delete(gotSpan, what)
						delete(wantSpan, what)
					} else {
						t.Errorf("span value %q for key %s is not valid", gotVal, what)
					}
				}
			}
		}
	}

	// do a diff on a generated map that sets values to * when the * check succeeded
	injectEndpoint(endpoint, wantSpan)
	if diff := cmp.Diff(wantSpan, gotSpan); diff != "" {
		t.Errorf("otel span info did not match fixture in %q (-want +got):\n%s", fixture.Filename, diff)
	}
}

// runOtelCli runs the a server and otel-cli together and captures their
// output as data to return for further testing.
// all failures are fatal, no point in testing if this is broken
func runOtelCli(t *testing.T, fixture Fixture) (string, cmd.StatusOutput, otlpserver.CliEvent, otlpserver.CliEventList) {
	// only supports 0 or 1 spans, which is fine for these tests
	// these channels need to be buffered or the callback will hang trying to send while
	// the main goroutine here is still running and waiting on otel-cli
	rcvSpan := make(chan otlpserver.CliEvent, 1)
	rcvEvents := make(chan otlpserver.CliEventList, 1)
	cb := func(span otlpserver.CliEvent, events otlpserver.CliEventList) bool {
		rcvSpan <- span
		rcvEvents <- events
		return true // tell the server we're done and it can exit its loop
	}

	cs := otlpserver.NewServer(cb, func(*otlpserver.Server) {})
	listener, err := net.Listen("tcp", "localhost:0")
	endpoint := listener.Addr().String()
	if err != nil {
		t.Fatalf("failed to listen on OTLP endpoint %q: %s", endpoint, err)
	}
	t.Logf("starting OTLP server on %q", endpoint)
	go func() {
		cs.ServeGPRC(listener)
	}()

	// TODO: figure out the best way to build the binary and detect if the build is stale
	// ^^ probably doesn't matter much in CI, can auto-build, but for local workflow it matters
	// TODO: also be able to pass args to otel-cli
	// TODO: also need to test other subcommands
	// TODO: does that imply all otel-cli commands should be able to dump status? e.g. otel-cli span --status
	args := []string{"status"}
	if len(fixture.Config.CliArgs) > 0 {
		for _, v := range fixture.Config.CliArgs {
			args = append(args, strings.ReplaceAll(v, "{{endpoint}}", endpoint))
		}
	}
	statusCmd := exec.Command("./otel-cli", args...)
	statusCmd.Env = mkEnviron(endpoint, fixture.Config.Env)
	// grab stderr & stdout comingled so that if otel-cli prints anything to either it's not
	// supposed to it will cause e.g. status json parsing and other tests to fail
	t.Logf("going to exec 'env -i %s ./otel-cli", strings.Join(statusCmd.Env, " "))
	statusOut, err := statusCmd.CombinedOutput()
	if err != nil {
		t.Log(string(statusOut))
		wd, _ := os.Getwd()
		t.Fatalf("Executing 'env -i %s %s/otel-cli failed: %s", strings.Join(statusCmd.Env, " "), wd, err)
	}

	status := cmd.StatusOutput{}
	err = json.Unmarshal(statusOut, &status)
	if err != nil {
		t.Fatalf("parsing otel-cli status output failed: %s", err)
	}

	// remove PATH from the output but only if it's exactly what we set on exec
	if path, ok := status.Env["PATH"]; ok {
		if path == minimumPath {
			delete(status.Env, "PATH")
		}
	}

	// grab the spans & events from the server off the channels it writes to
	var retSpan otlpserver.CliEvent
	var retEvents otlpserver.CliEventList
	if fixture.SpansExpected > 0 {
		retSpan = <-rcvSpan
		retEvents = <-rcvEvents
	}

	cs.Stop()

	return endpoint, status, retSpan, retEvents
}

// mkEnviron converts a string map to a list of k=v strings.
func mkEnviron(endpoint string, env map[string]string) []string {
	mapped := make([]string, len(env)+1)
	var i int
	for k, v := range env {
		mapped[i] = k + "=" + strings.ReplaceAll(v, "{{endpoint}}", endpoint)
		i++
	}

	// always tack on a PATH otherwise the binary will fail with no PATH
	// to get to getent(1)
	mapped[len(mapped)-1] = "PATH=" + minimumPath

	return mapped
}

// injectEndpoint iterates over the map and updates the values, replacing all instances
// of {{endpoint}} with the provided endpoint. This is needed because the otlpserver is
// configured to listen on :0 which has it grab a random port. Once that's generated we
// need to inject it into all the values so the test comparisons work as expected.
func injectEndpoint(endpoint string, target map[string]string) {
	for k, v := range target {
		target[k] = strings.ReplaceAll(v, "{{endpoint}}", endpoint)
	}
}
