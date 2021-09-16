package main

// end-to-end tests for otel-cli using json test definitions in ./fixtures

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/equinix-labs/otel-cli/cmd"
	"github.com/equinix-labs/otel-cli/otlpserver"
	"github.com/google/go-cmp/cmp"
)

// otel-cli will fail with "getent not found" if PATH is empty
// so set it to the bare minimum and always the same for cleanup
const minimumPath = `/bin`
const defaultTestTimeout = time.Second

type FixtureConfig struct {
	CliArgs []string `json:"cli_args"`
	Env     map[string]string
	// timeout for how long to wait for the whole test in failure cases
	TestTimeoutMs int `json:"test_timeout_ms"`
	// when true this test will be excluded under go -test.short mode
	IsLongTest bool `json:"is_long_test"`
	// for timeout tests we need to start the server to generate the endpoint
	// but do not want it to answer when otel-cli calls, this does that
	StopServerBeforeExec bool `json:"stop_server_before_exec"`
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
		// some tests explicitly spend time sleeping/waiting to validate timeouts are working
		// and when they are marked as such, they can be skipped with go test -test.short
		if testing.Short() && fixture.Config.IsLongTest {
			t.Skip("skipping timeout tests in short mode")
			continue
		}

		// sets up an OTLP server, runs otel-cli, packages data up in these return vals
		endpoint, results, span, events := runOtelCli(t, fixture)

		// check timeout and process status expectations
		checkProcess(t, fixture, results)

		// compares the spans from the server against expectations in the fixture
		checkSpanData(t, fixture, endpoint, span, events)

		// many of the basic plumbing tests use status so it has its own set of checks
		// but these shouldn't run for testing the other subcommands
		if len(fixture.Config.CliArgs) > 0 && fixture.Config.CliArgs[0] == "status" {
			checkStatusData(t, fixture, endpoint, results)
		} else {
			// checking the text output only makes sense for non-status paths
			checkOutput(t, fixture, endpoint, results)
		}
	}
}

func checkProcess(t *testing.T, fixture Fixture, results Results) {
	if results.TimedOut != fixture.Expect.TimedOut {
		t.Errorf("[%s] test timeout status is %t but expected %t", fixture.Filename, results.TimedOut, fixture.Expect.TimedOut)
	}
	if results.CommandFailed != fixture.Expect.CommandFailed {
		t.Errorf("[%s] command failed is %t but expected %t", fixture.Filename, results.CommandFailed, fixture.Expect.CommandFailed)
	}
}

// checkOutput looks that otel-cli output stored in the results and compares against
// the fixture expectation (with {{endpoint}} replaced).
func checkOutput(t *testing.T, fixture Fixture, endpoint string, results Results) {
	wantOutput := strings.ReplaceAll(fixture.Expect.CliOutput, "{{endpoint}}", endpoint)
	if diff := cmp.Diff(wantOutput, results.CliOutput); diff != "" {
		t.Errorf("otel-cli output did not match fixture in %q (-want +got):\n%s", fixture.Filename, diff)
	}
}

// checkStatusData compares the sections of otel-cli status output against
// fixture data, substituting {{endpoint}} into fixture data as needed.
func checkStatusData(t *testing.T, fixture Fixture, endpoint string, results Results) {
	// check the env
	injectEndpoint(endpoint, fixture.Expect.Env)
	if diff := cmp.Diff(fixture.Expect.Env, results.Env); diff != "" {
		t.Errorf("env data did not match fixture in %q (-want +got):\n%s", fixture.Filename, diff)
	}

	// check diagnostics, use string maps so the diff output is easy to compare to json
	wantDiag := fixture.Expect.Diagnostics.ToStringMap()
	gotDiag := results.Diagnostics.ToStringMap()
	injectEndpoint(endpoint, wantDiag)
	// there's almost always going to be cli_args in practice, so if the fixture has
	// an empty string, just ignore that key
	if wantDiag["cli_args"] == "" {
		gotDiag["cli_args"] = ""
	}
	if diff := cmp.Diff(wantDiag, gotDiag); diff != "" {
		t.Errorf("[%s] diagnostic data did not match fixture (-want +got):\n%s", fixture.Filename, diff)
	}

	// check the configuration
	wantConf := fixture.Expect.Config.ToStringMap()
	gotConf := results.Config.ToStringMap()
	injectEndpoint(endpoint, wantConf)
	if diff := cmp.Diff(wantConf, gotConf); diff != "" {
		t.Errorf("[%s] config data did not match fixture (-want +got):\n%s", fixture.Filename, diff)
	}
}

// checkSpanData compares the data in spans received from otel-cli against the
// fixture data.
func checkSpanData(t *testing.T, fixture Fixture, endpoint string, span otlpserver.CliEvent, events otlpserver.CliEventList) {
	// check the expected span data against what was received by the OTLP server
	gotSpan := span.ToStringMap()
	injectEndpoint(endpoint, gotSpan)
	// remove keys that aren't supported for comparison (for now)
	delete(gotSpan, "is_populated")
	delete(gotSpan, "library")
	wantSpan := map[string]string{} // to be passed to cmp.Diff
	for what, re := range map[string]*regexp.Regexp{
		"trace_id":   regexp.MustCompile(`^[0-9a-fA-F]{32}$`),
		"span_id":    regexp.MustCompile(`^[0-9a-fA-F]{16}$`),
		"name":       regexp.MustCompile(`^\w+$`),
		"parent":     regexp.MustCompile(`^[0-9a-fA-F]{32}$`),
		"kind":       regexp.MustCompile(`^\w+$`), // TODO: can validate more here
		"start":      regexp.MustCompile(`^\d+$`),
		"end":        regexp.MustCompile(`^\d+$`),
		"attributes": regexp.MustCompile(`\w+=.+`), // not great but should validate at least one k=v
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
						t.Errorf("[%s] span value %q for key %s is not valid", fixture.Filename, gotVal, what)
					}
				}
			}
		}
	}

	// do a diff on a generated map that sets values to * when the * check succeeded
	injectEndpoint(endpoint, wantSpan)
	if diff := cmp.Diff(wantSpan, gotSpan); diff != "" {
		t.Errorf("[%s] otel span info did not match fixture (-want +got):\n%s", fixture.Filename, diff)
	}
}

// runOtelCli runs the a server and otel-cli together and captures their
// output as data to return for further testing.
func runOtelCli(t *testing.T, fixture Fixture) (string, Results, otlpserver.CliEvent, otlpserver.CliEventList) {
	started := time.Now()

	var results Results
	var retSpan otlpserver.CliEvent
	var retEvents otlpserver.CliEventList

	// these channels need to be buffered or the callback will hang trying to send
	rcvSpan := make(chan otlpserver.CliEvent, 100) // 100 spans is enough for anybody
	rcvEvents := make(chan otlpserver.CliEventList, 100)

	// otlpserver calls this function for each span received
	cb := func(span otlpserver.CliEvent, events otlpserver.CliEventList) bool {
		rcvSpan <- span
		rcvEvents <- events

		results.Spans++
		results.Events += len(events)

		// true tells the server we're done and it can exit its loop
		return results.Spans >= fixture.Expect.Spans
	}

	cs := otlpserver.NewServer(cb, func(*otlpserver.Server) {})
	defer cs.Stop()

	serverTimeout := time.Duration(fixture.Config.TestTimeoutMs) * time.Millisecond
	if serverTimeout == time.Duration(0) {
		serverTimeout = defaultTestTimeout
	}

	// start a timeout goroutine for the otlp server, cancelable with done<-struct{}{}
	cancelServerTimeout := make(chan struct{}, 1)
	go func() {
		select {
		case <-time.After(serverTimeout):
			results.TimedOut = true
			cs.Stop() // supports multiple calls
		case <-cancelServerTimeout:
			return
		}
	}()

	// port :0 means randomly assigned port, which we copy into {{endpoint}}
	listener, err := net.Listen("tcp", "localhost:0")
	endpoint := listener.Addr().String()
	if err != nil {
		t.Fatalf("[%s] failed to listen on OTLP endpoint %q: %s", fixture.Filename, endpoint, err)
	}
	t.Logf("[%s] starting OTLP server on %q", fixture.Filename, endpoint)

	// TODO: might be neat to have a mode where we start the listener and do nothing
	// with it to simulate a hung server or opentelemetry-collector
	go func() {
		cs.ServeGPRC(listener)
	}()

	// let things go this far to generate the endpoint port then stop the server before
	// calling otel-cli so we can test timeouts
	if fixture.Config.StopServerBeforeExec {
		cs.Stop()
		listener.Close()
	}

	// TODO: figure out the best way to build the binary and detect if the build is stale
	// ^^ probably doesn't matter much in CI, can auto-build, but for local workflow it matters
	// TODO: should all otel-cli commands be able to dump status? e.g. otel-cli span --status
	args := []string{}
	if len(fixture.Config.CliArgs) > 0 {
		for _, v := range fixture.Config.CliArgs {
			args = append(args, strings.ReplaceAll(v, "{{endpoint}}", endpoint))
		}
	}
	statusCmd := exec.Command("./otel-cli", args...)
	statusCmd.Env = mkEnviron(endpoint, fixture.Config.Env)

	cancelProcessTimeout := make(chan struct{}, 1)
	go func() {
		select {
		case <-time.After(serverTimeout):
			results.TimedOut = true
			err = statusCmd.Process.Kill()
			if err != nil {
				// TODO: this might be a bit fragle, soften this up later if it ends up problematic
				log.Fatalf("[%s] process kill failed: %s", fixture.Filename, err)
			}
		case <-cancelProcessTimeout:
			return
		}
	}()

	// grab stderr & stdout comingled so that if otel-cli prints anything to either it's not
	// supposed to it will cause e.g. status json parsing and other tests to fail
	t.Logf("[%s] going to exec 'env -i %s ./otel-cli %s'", fixture.Filename, strings.Join(statusCmd.Env, " "), strings.Join(args, " "))
	cliOut, err := statusCmd.CombinedOutput()
	results.CliOutput = string(cliOut)
	if err != nil {
		results.CommandFailed = true
		t.Logf("[%s] executing command failed: %s", fixture.Filename, err)
	}

	// send stop signals to the timeouts and OTLP server
	cancelProcessTimeout <- struct{}{}
	cancelServerTimeout <- struct{}{}
	cs.Stop()

	// only try to parse status json if it was a status command
	// TODO: support variations on otel-cli where status isn't the first arg
	if len(cliOut) > 0 && len(args) > 0 && args[0] == "status" {
		err = json.Unmarshal(cliOut, &results)
		if err != nil {
			t.Fatalf("[%s] parsing otel-cli status output failed: %s", fixture.Filename, err)
		}

		// remove PATH from the output but only if it's exactly what we set on exec
		if path, ok := results.Env["PATH"]; ok {
			if path == minimumPath {
				delete(results.Env, "PATH")
			}
		}
	}

	// when no spans are expected, return without reading from the channels
	if fixture.Expect.Spans == 0 {
		return endpoint, results, retSpan, retEvents
	}

	// grab the spans & events from the server off the channels it writes to
	remainingTimeout := serverTimeout - time.Since(started)
	var gatheredSpans int
gather:
	for {
		select {
		case <-time.After(remainingTimeout):
			break gather
		case retSpan = <-rcvSpan:
			// events is always populated at the same time as the span is sent
			// and will always send at least an empty list
			retEvents = <-rcvEvents

			// with this approach, any mismatch in spans produced and expected results
			// in a timeout with the above time.After
			gatheredSpans++
			if gatheredSpans == results.Spans {
				break gather
			}
		}
	}

	return endpoint, results, retSpan, retEvents
}

// mkEnviron converts a string map to a list of k=v strings and tacks on PATH.
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
