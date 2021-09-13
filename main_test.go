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
	Name          string           `json:"name"`
	Filename      string           `json:"-"`
	Config        FixtureConfig    `json:"config"`
	Expect        cmd.StatusOutput `json:"expect"`
	SpansExpected int              `json:"spans_expected"`
	Timeout       int              `json:"timeout"`
	ShouldTimeout bool             `json:"should_timeout"` // otel connection stub->cli should fail
}

// EventsOut stores the output from the otlpserver in a tid/sid/event structure.
//                trace id   span id
type EventsOut map[string]map[string]otlpserver.CliEvent

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
		status, events := runOtelCli(t, fixture)
		checkData(t, fixture, status, events)
	}
}

// checkData takes the data returned from otel-cli status and compares it to the
// fixture data and fails the tests if anything doesn't match.
func checkData(t *testing.T, fixture Fixture, status cmd.StatusOutput, events EventsOut) {
	// check the env
	if diff := cmp.Diff(fixture.Expect.Env, status.Env); diff != "" {
		t.Errorf("env data did not match fixture in %q (-want +got):\n%s", fixture.Filename, diff)
	}

	// TODO: this needs might need a revamp of config defaults to move them from Cobra
	// to a version of the Config struct with the defaults so they can be checked
	//if diff := cmp.Diff(fixture.Expect.Config, status.Config); diff != "" {
	//	t.Errorf("config data did not match fixture in %q (-want +got):\n%s", fixture.Filename, diff)
	//}

	// check the otel span values
	// find usages of *, do the check on the status data manually, and set up cmpSpan
	gotSpan := map[string]string{}  // to be passed to cmp.Diff
	wantSpan := map[string]string{} // to be passed to cmp.Diff
	for what, re := range map[string]*regexp.Regexp{
		"trace_id":    regexp.MustCompile("^[0-9a-fA-F]{32}$"),
		"span_id":     regexp.MustCompile("^[0-9a-fA-F]{16}$"),
		"is_sampled":  regexp.MustCompile("^true|false$"),
		"trace_flags": regexp.MustCompile("^[0-9]{2}$"),
	} {
		if wantVal, ok := fixture.Expect.SpanData[what]; ok {
			wantSpan[what] = wantVal // make a straight copy to make cmp.Diff happy
			if gotVal, ok := status.SpanData[what]; ok {
				gotSpan[what] = gotVal // default to the existing value
				if wantVal == "*" {
					if re.MatchString(gotVal) {
						gotSpan[what] = "*" // success!, make the Cmp test succeed
					} else {
						t.Errorf("span value %q for key %s is not valid", wantVal, what)
					}
				}
			}
		}
	}

	// do a diff on a generated map that sets values to * when the * check succeeded
	if diff := cmp.Diff(gotSpan, wantSpan); diff != "" {
		t.Logf("gotSpan: %q", gotSpan)
		t.Logf("wantSpan: %q", wantSpan)
		t.Errorf("otel span info did not match fixture in %q (-want +got):\n%s", fixture.Filename, diff)
	}
}

// runOtelCli runs the a server and otel-cli together and captures their
// output as data to return for further testing.
// all failures are fatal, no point in testing if this is broken
func runOtelCli(t *testing.T, fixture Fixture) (cmd.StatusOutput, EventsOut) {
	stop := func(*otlpserver.Server) {}

	rcvSpan := make(chan otlpserver.CliEvent)
	//rcvEvents := make(chan otlpserver.CliEventList)
	cb := func(span otlpserver.CliEvent, events otlpserver.CliEventList) bool {
		rcvSpan <- span
		//rcvEvents <- events
		return false
	}

	// TODO: find a way to do random ports?
	cs := otlpserver.NewServer(cb, stop)
	listener, err := net.Listen("tcp", "localhost:7777")
	if err != nil {
		t.Fatalf("failed to listen on OTLP endpoint %q: %s", "localhost:7777", err)
	}
	go func() {
		cs.ServeGPRC(listener) // TODO: generate this with random port value instead
	}()

	// TODO: figure out the best way to build the binary and detect if the build is stale
	// ^^ probably doesn't matter much in CI, can auto-build, but for local workflow it matters
	// TODO: also be able to pass args to otel-cli
	// TODO: also need to test other subcommands
	// TODO: does that imply all otel-cli commands should be able to dump status? e.g. otel-cli span --status
	statusCmd := exec.Command("./otel-cli", "status")
	statusCmd.Env = mkEnviron(fixture.Config.Env)
	statusOut, err := statusCmd.Output()
	if err != nil {
		t.Log(statusOut)
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

	// TODO: make this accept multiple spans
	span := otlpserver.CliEvent{}
	if fixture.SpansExpected > 0 {
		span = <-rcvSpan
	}

	eo := make(EventsOut)
	eo[span.TraceID] = map[string]otlpserver.CliEvent{
		span.SpanID: {},
	}

	cs.Stop()

	return status, eo
}

// mkEnviron converts a string map to a list of k=v strings.
func mkEnviron(env map[string]string) []string {
	mapped := make([]string, len(env)+1)
	var i int
	for k, v := range env {
		mapped[i] = k + "=" + v
		i++
	}

	mapped[len(mapped)-1] = "PATH=" + minimumPath

	return mapped
}
