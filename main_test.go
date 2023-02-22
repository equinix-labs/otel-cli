package main_test

// implements the data-driven tests of otel-cli using data in data_for_test.go

// TODO: stop using fixture.Name to track foreground/background

import (
	"crypto/tls"
	"encoding/json"
	"log"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/equinix-labs/otel-cli/otlpserver"
	"github.com/google/go-cmp/cmp"
)

// otel-cli will fail with "getent not found" if PATH is empty
// so set it to the bare minimum and always the same for cleanup
const minimumPath = `/bin:/usr/bin`
const defaultTestTimeout = time.Second

func TestMain(m *testing.M) {
	// wipe out this process's envvars right away to avoid pollution & leakage
	os.Clearenv()
	result := m.Run()
	os.Exit(result)
}

// TestOtelCli loads all the json fixtures and executes the tests.
func TestOtelCli(t *testing.T) {
	_, err := os.Stat("./otel-cli")
	if os.IsNotExist(err) {
		t.Fatalf("otel-cli must be built and present as ./otel-cli for this test suite to work (try: go build)")
	}

	var fixtureCount int
	for _, suite := range suites {
		fixtureCount += len(suite)
		for i, fixture := range suite {
			// clean up some nils here so the test data can be a bit more terse
			if fixture.Config.CliArgs == nil {
				suite[i].Config.CliArgs = []string{}
			}
			if fixture.Config.Env == nil {
				suite[i].Config.Env = map[string]string{}
			}
			if fixture.Expect.Env == nil {
				suite[i].Expect.Env = map[string]string{}
			}
			if fixture.Expect.SpanData == nil {
				suite[i].Expect.SpanData = map[string]string{}
			}
			// make sure PATH hasn't been set, because doing that in fixtures is naughty
			if _, ok := fixture.Config.Env["PATH"]; ok {
				t.Fatalf("fixture in file %s is not allowed to modify or test envvar PATH", fixture.Name)
			}
		}
	}

	t.Logf("Running %d test suites and %d fixtures.", len(suites), fixtureCount)
	if len(suites) == 0 || fixtureCount == 0 {
		t.Fatal("no test fixtures loaded!")
	}

	// generates a CA, client, and server certs to use in tests
	tlsData := generateTLSData(t)
	defer tlsData.cleanup()

	for _, suite := range suites {
		// a fixture can be backgrounded after starting it up for e.g. otel-cli span background
		// a second fixture with the same description later in the list will "foreground" it
		bgFixtureWaits := make(map[string]chan struct{})
		bgFixtureDones := make(map[string]chan struct{})

	fixtures:
		for _, fixture := range suite {
			// some tests explicitly spend time sleeping/waiting to validate timeouts are working
			// and when they are marked as such, they can be skipped with go test -test.short
			if testing.Short() && fixture.Config.IsLongTest {
				t.Skipf("[%s] skipping timeout tests in short mode", fixture.Name)
				continue fixtures
			}

			// when a fixture is foregrounded all it does is signal the background fixture
			// to finish doing its then, waits for it to finish, then continues on
			if fixture.Config.Foreground {
				if wait, ok := bgFixtureWaits[fixture.Name]; ok {
					wait <- struct{}{}
					delete(bgFixtureWaits, fixture.Name)
				} else {
					t.Fatalf("BUG in test or fixture: unexpected foreground fixture wait chan named %q", fixture.Name)
				}
				if done, ok := bgFixtureDones[fixture.Name]; ok {
					<-done
					delete(bgFixtureDones, fixture.Name)
				} else {
					t.Fatalf("BUG in test or fixture: unexpected foreground fixture done chan named %q", fixture.Name)
				}

				continue fixtures
			}

			// flow control for backgrounding fixtures:
			fixtureWait := make(chan struct{})
			fixtureDone := make(chan struct{})

			go runFixture(t, fixture, fixtureWait, fixtureDone, tlsData)

			if fixture.Config.Background {
				// save off the channels for flow control
				t.Logf("[%s] fixture %q backgrounded", fixture.Name, fixture.Name)
				bgFixtureWaits[fixture.Name] = fixtureWait
				bgFixtureDones[fixture.Name] = fixtureDone
			} else {
				// actually the default case, just block as if the code was ran synchronously
				fixtureWait <- struct{}{}
				<-fixtureDone
			}
		}
	}
}

// runFixture runs the OTLP server & command, waits for signal, checks
// results, then signals it's done.
func runFixture(t *testing.T, fixture Fixture, wait, done chan struct{}, tlsData tlsHelpers) {
	// sets up an OTLP server, runs otel-cli, packages data up in these return vals
	endpoint, results, span, events := runOtelCli(t, fixture, tlsData)
	<-wait
	checkAll(t, fixture, endpoint, results, span, events, tlsData)
	done <- struct{}{}
}

// checkAll gathers up all the check* funcs below into one function.
func checkAll(t *testing.T, fixture Fixture, endpoint string, results Results, span otlpserver.CliEvent, events otlpserver.CliEventList, tlsData tlsHelpers) {
	// check timeout and process status expectations
	checkProcess(t, fixture, results)

	// compare the number of recorded spans against expectations in the fixture
	checkSpanCount(t, fixture, results)

	// compares the data in each recorded span against expectations in the fixture
	checkSpanData(t, fixture, endpoint, span, events, tlsData)

	// many of the basic plumbing tests use status so it has its own set of checks
	// but these shouldn't run for testing the other subcommands
	if len(fixture.Config.CliArgs) > 0 && fixture.Config.CliArgs[0] == "status" && !fixture.Expect.CommandFailed {
		checkStatusData(t, fixture, endpoint, results, tlsData)
	} else {
		// checking the text output only makes sense for non-status paths
		checkOutput(t, fixture, endpoint, results, tlsData)
	}
}

// compare the number of spans recorded by the test server against the
// number of expected spans in the fixture, if specified. If no expected
// span count is specified, this check always passes.
func checkSpanCount(t *testing.T, fixture Fixture, results Results) {
	if results.Spans != fixture.Expect.Spans {
		t.Errorf("[%s] span count was %d but expected %d", fixture.Name, results.Spans, fixture.Expect.Spans)
	}
}

// checkProcess validates configured expectations about whether otel-cli failed
// or the test timed out. These are mostly used for testing that otel-cli fails
// in the way we want it to.
func checkProcess(t *testing.T, fixture Fixture, results Results) {
	if results.TimedOut != fixture.Expect.TimedOut {
		t.Errorf("[%s] test timeout status is %t but expected %t", fixture.Name, results.TimedOut, fixture.Expect.TimedOut)
	}
	if results.CommandFailed != fixture.Expect.CommandFailed {
		t.Errorf("[%s] command failed is %t but expected %t", fixture.Name, results.CommandFailed, fixture.Expect.CommandFailed)
	}
}

// checkOutput looks that otel-cli output stored in the results and compares against
// the fixture expectation (with {{endpoint}} replaced).
func checkOutput(t *testing.T, fixture Fixture, endpoint string, results Results, tlsData tlsHelpers) {
	wantOutput := injectVars(fixture.Expect.CliOutput, endpoint, tlsData)
	gotOutput := results.CliOutput
	if fixture.Expect.CliOutputRe != nil {
		gotOutput = fixture.Expect.CliOutputRe.ReplaceAllString(gotOutput, "")
	}
	if diff := cmp.Diff(wantOutput, gotOutput); diff != "" {
		if fixture.Expect.CliOutputRe != nil {
			t.Errorf("[%s] test fixture RE modified output from %q to %q", fixture.Name, fixture.Expect.CliOutput, gotOutput)
		}
		t.Errorf("[%s] otel-cli output did not match fixture (-want +got):\n%s", fixture.Name, diff)
	}
}

// checkStatusData compares the sections of otel-cli status output against
// fixture data, substituting {{endpoint}} into fixture data as needed.
func checkStatusData(t *testing.T, fixture Fixture, endpoint string, results Results, tlsData tlsHelpers) {
	// check the env
	injectMapVars(endpoint, fixture.Expect.Env, tlsData)
	if diff := cmp.Diff(fixture.Expect.Env, results.Env); diff != "" {
		t.Errorf("env data did not match fixture in %q (-want +got):\n%s", fixture.Name, diff)
	}

	// check diagnostics, use string maps so the diff output is easy to compare to json
	wantDiag := fixture.Expect.Diagnostics.ToStringMap()
	gotDiag := results.Diagnostics.ToStringMap()
	injectMapVars(endpoint, wantDiag, tlsData)
	// there's almost always going to be cli_args in practice, so if the fixture has
	// an empty string, just ignore that key
	if wantDiag["cli_args"] == "" {
		gotDiag["cli_args"] = ""
	}
	if diff := cmp.Diff(wantDiag, gotDiag); diff != "" {
		t.Errorf("[%s] diagnostic data did not match fixture (-want +got):\n%s", fixture.Name, diff)
	}

	// check the configuration
	wantConf := fixture.Expect.Config.ToStringMap()
	gotConf := results.Config.ToStringMap()
	// if an expected config string is set to "*" it will match anything
	// and is effectively ignored
	for k, v := range wantConf {
		if v == "*" {
			// set to same so cmd.Diff will ignore
			wantConf[k] = gotConf[k]
		}
	}
	injectMapVars(endpoint, wantConf, tlsData)
	if diff := cmp.Diff(wantConf, gotConf); diff != "" {
		t.Errorf("[%s] config data did not match fixture (-want +got):\n%s", fixture.Name, diff)
	}
}

// spanRegexChecks is a map of field names and regexes for basic presence
// and validity checking of span data fields with expected set to "*"
var spanRegexChecks = map[string]*regexp.Regexp{
	"trace_id":   regexp.MustCompile(`^[0-9a-fA-F]{32}$`),
	"span_id":    regexp.MustCompile(`^[0-9a-fA-F]{16}$`),
	"name":       regexp.MustCompile(`^\w+$`),
	"parent":     regexp.MustCompile(`^[0-9a-fA-F]{32}$`),
	"kind":       regexp.MustCompile(`^\w+$`), // TODO: can validate more here
	"start":      regexp.MustCompile(`^\d+$`),
	"end":        regexp.MustCompile(`^\d+$`),
	"attributes": regexp.MustCompile(`\w+=.+`), // not great but should validate at least one k=v
}

// checkSpanData compares the data in spans received from otel-cli against the
// fixture data.
func checkSpanData(t *testing.T, fixture Fixture, endpoint string, span otlpserver.CliEvent, events otlpserver.CliEventList, tlsData tlsHelpers) {
	// check the expected span data against what was received by the OTLP server
	gotSpan := span.ToStringMap()
	injectMapVars(endpoint, gotSpan, tlsData)
	// remove keys that aren't supported for comparison (for now)
	delete(gotSpan, "is_populated")
	delete(gotSpan, "library")
	wantSpan := map[string]string{} // to be passed to cmp.Diff

	// verify all fields that were expected were present in output span
	for what := range fixture.Expect.SpanData {
		if _, ok := gotSpan[what]; !ok {
			t.Errorf("[%s] expected span to have key %q but it is not present", fixture.Name, what)
		}
	}

	// go over all the keys in the received span and check against expected values
	// in the fixture, and the spanRegexChecks
	// modifies the gotSpan/wantSpan maps so cmp.Diff can do its thing
	for what, gotVal := range gotSpan {
		var wantVal string
		var ok bool
		if wantVal, ok = fixture.Expect.SpanData[what]; ok {
			wantSpan[what] = wantVal
		} else {
			wantSpan[what] = gotVal // make a straight copy to make cmp.Diff happy
		}

		if re, ok := spanRegexChecks[what]; ok {
			// * means if the above RE returns cleanly then pass the test
			if wantVal == "*" {
				if re.MatchString(gotVal) {
					delete(gotSpan, what)
					delete(wantSpan, what)
				} else {
					t.Errorf("[%s] span value %q for key %s is not valid", fixture.Name, gotVal, what)
				}
			}
		}
	}

	// do a diff on a generated map that sets values to * when the * check succeeded
	injectMapVars(endpoint, wantSpan, tlsData)
	if diff := cmp.Diff(wantSpan, gotSpan); diff != "" {
		t.Errorf("[%s] otel span info did not match fixture (-want +got):\n%s", fixture.Name, diff)
	}
}

// runOtelCli runs the a server and otel-cli together and captures their
// output as data to return for further testing.
func runOtelCli(t *testing.T, fixture Fixture, tlsData tlsHelpers) (string, Results, otlpserver.CliEvent, otlpserver.CliEventList) {
	started := time.Now()

	results := Results{
		SpanData: map[string]string{},
		Env:      map[string]string{},
	}
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

	var cs otlpserver.OtlpServer
	switch fixture.Config.ServerProtocol {
	case grpcProtocol:
		cs = otlpserver.NewServer("grpc", cb, func(otlpserver.OtlpServer) {})
	case httpProtocol:
		cs = otlpserver.NewServer("http", cb, func(otlpserver.OtlpServer) {})
	}
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
	var listener net.Listener
	var err error
	if fixture.Config.ServerTLSEnabled {
		tlsConf := *tlsData.serverTLSConf
		if fixture.Config.ServerTLSAuthEnabled {
			tlsConf.ClientAuth = tls.RequireAndVerifyClientCert
		}
		listener, err = tls.Listen("tcp", "localhost:0", &tlsConf)
	} else {
		listener, err = net.Listen("tcp", "localhost:0")
	}
	endpoint := listener.Addr().String()
	if err != nil {
		// t.Fatalf is not allowed since we run this in a goroutine
		t.Errorf("[%s] failed to listen on OTLP endpoint %q: %s", fixture.Name, endpoint, err)
		return endpoint, results, retSpan, retEvents
	}
	t.Logf("[%s] starting OTLP server on %q", fixture.Name, endpoint)

	// TODO: might be neat to have a mode where we start the listener and do nothing
	// with it to simulate a hung server or opentelemetry-collector
	go func() {
		cs.Serve(listener)
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
			args = append(args, injectVars(v, endpoint, tlsData))
		}
	}
	statusCmd := exec.Command("./otel-cli", args...)
	statusCmd.Env = mkEnviron(endpoint, fixture.Config.Env, tlsData)

	cancelProcessTimeout := make(chan struct{}, 1)
	go func() {
		select {
		case <-time.After(serverTimeout):
			results.TimedOut = true
			err = statusCmd.Process.Kill()
			if err != nil {
				// TODO: this might be a bit fragle, soften this up later if it ends up problematic
				log.Fatalf("[%s] process kill failed: %s", fixture.Name, err)
			}
		case <-cancelProcessTimeout:
			return
		}
	}()

	// grab stderr & stdout comingled so that if otel-cli prints anything to either it's not
	// supposed to it will cause e.g. status json parsing and other tests to fail
	t.Logf("[%s] going to exec 'env -i %s %s'", fixture.Name, strings.Join(statusCmd.Env, " "), strings.Join(statusCmd.Args, " "))
	cliOut, err := statusCmd.CombinedOutput()
	results.CliOutput = string(cliOut)
	if err != nil {
		results.CommandFailed = true
		t.Logf("[%s] executing command failed: %s", fixture.Name, err)
	}

	// send stop signals to the timeouts and OTLP server
	cancelProcessTimeout <- struct{}{}
	cancelServerTimeout <- struct{}{}
	cs.Stop()

	// only try to parse status json if it was a status command
	// TODO: support variations on otel-cli where status isn't the first arg
	if len(args) > 0 && args[0] == "status" && !fixture.Expect.CommandFailed {
		err = json.Unmarshal(cliOut, &results)
		if err != nil {
			t.Errorf("[%s] parsing otel-cli status output failed: %s", fixture.Name, err)
			t.Logf("[%s] output received: %q", fixture.Name, cliOut)
			return endpoint, results, retSpan, retEvents
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
				// TODO: it would be slightly nicer to use plural.Selectf instead of 'span(s)'
				t.Logf("[%s] test gathered %d span(s)", fixture.Name, gatheredSpans)
				break gather
			}
		}
	}

	return endpoint, results, retSpan, retEvents
}

// mkEnviron converts a string map to a list of k=v strings and tacks on PATH.
func mkEnviron(endpoint string, env map[string]string, tlsData tlsHelpers) []string {
	mapped := make([]string, len(env)+1)
	var i int
	for k, v := range env {
		mapped[i] = k + "=" + injectVars(v, endpoint, tlsData)
		i++
	}

	// always tack on a PATH otherwise the binary will fail with no PATH
	// to get to getent(1)
	mapped[len(mapped)-1] = "PATH=" + minimumPath

	return mapped
}

// injectMapVars iterates over the map and updates the values, replacing all instances
// of {{endpoint}}, {{tls_ca_cert}}, {{tls_client_cert}}, and {{tls_client_key}} with
// test values.
func injectMapVars(endpoint string, target map[string]string, tlsData tlsHelpers) {
	for k, v := range target {
		target[k] = injectVars(v, endpoint, tlsData)
	}
}

// injectVars replaces all instances of {{endpoint}}, {{tls_ca_cert}},
// {{tls_client_cert}}, and {{tls_client_key}} with test values.
// This is needed because the otlpserver is configured to listen on :0 which has it
// grab a random port. Once that's generated we need to inject it into all the values
// so the test comparisons work as expected. Similarly for TLS testing, a temp CA and
// certs are created and need to be injected.
func injectVars(in, endpoint string, tlsData tlsHelpers) string {
	out := strings.ReplaceAll(in, "{{endpoint}}", endpoint)
	out = strings.ReplaceAll(out, "{{tls_ca_cert}}", tlsData.caFile)
	out = strings.ReplaceAll(out, "{{tls_client_cert}}", tlsData.clientFile)
	out = strings.ReplaceAll(out, "{{tls_client_key}}", tlsData.clientPrivKeyFile)
	return out
}
