package main_test

// This file implements the data-driven test harness for otel-cli. It executes
// tests defined in data_for_test.go, and uses the CA implementation in
// tls_for_test.go.
//
// see TESTING.md for details

import (
	"bytes"
	"context"
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

	"github.com/equinix-labs/otel-cli/otlpclient"
	"github.com/equinix-labs/otel-cli/otlpserver"
	"github.com/google/go-cmp/cmp"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
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

// TestOtelCli iterates over all defined fixtures and executes the tests.
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

			// inject the TlsData into the fixture
			fixture.TlsData = tlsData

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

			go runFixture(t, fixture, fixtureWait, fixtureDone)

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
func runFixture(t *testing.T, fixture Fixture, wait, done chan struct{}) {
	// sets up an OTLP server, runs otel-cli, packages data up in these return vals
	endpoint, results := runOtelCli(t, fixture)
	<-wait

	// inject the runtime endpoint into the fixture
	fixture.Endpoint = endpoint
	checkAll(t, fixture, results)
	done <- struct{}{}
}

// checkAll gathers up all the check* funcs below into one function.
func checkAll(t *testing.T, fixture Fixture, results Results) {
	// check timeout and process status expectations
	success := checkProcess(t, fixture, results)
	// when the process fails, no point in checking the rest, it's just noise
	if !success {
		t.Log("otel-cli process failed unexpectedly, will not test values from it")
		return
	}

	// compare the number of recorded spans against expectations in the fixture
	checkSpanCount(t, fixture, results)

	// compares the data in each recorded span against expectations in the fixture
	if len(fixture.Expect.SpanData) > 0 {
		checkSpanData(t, fixture, results)
	}

	// many of the basic plumbing tests use status so it has its own set of checks
	// but these shouldn't run for testing the other subcommands
	if len(fixture.Config.CliArgs) > 0 && fixture.Config.CliArgs[0] == "status" && results.ExitCode == 0 {
		checkStatusData(t, fixture, results)
	} else {
		// checking the text output only makes sense for non-status paths
		checkOutput(t, fixture, results)
	}

	if len(fixture.Expect.Headers) > 0 {
		checkHeaders(t, fixture, results)
	}

	if len(fixture.Expect.ServerMeta) > 0 {
		checkServerMeta(t, fixture, results)
	}

	checkFuncs(t, fixture, results)
}

// compare the number of spans recorded by the test server against the
// number of expected spans in the fixture, if specified. If no expected
// span count is specified, this check always passes.
func checkSpanCount(t *testing.T, fixture Fixture, results Results) {
	if results.SpanCount != fixture.Expect.SpanCount {
		t.Errorf("[%s] span count was %d but expected %d", fixture.Name, results.SpanCount, fixture.Expect.SpanCount)
	}
}

// checkProcess validates configured expectations about whether otel-cli failed
// or the test timed out. These are mostly used for testing that otel-cli fails
// in the way we want it to.
func checkProcess(t *testing.T, fixture Fixture, results Results) bool {
	if results.TimedOut != fixture.Expect.TimedOut {
		t.Errorf("[%s] test timeout status is %t but expected %t", fixture.Name, results.TimedOut, fixture.Expect.TimedOut)
		return false
	}
	if results.CommandFailed != fixture.Expect.CommandFailed {
		t.Errorf("[%s] command failed is %t but expected %t", fixture.Name, results.CommandFailed, fixture.Expect.CommandFailed)
		return false
	}
	return true
}

// checkOutput looks that otel-cli output stored in the results and compares against
// the fixture expectation (with {{endpoint}} replaced).
func checkOutput(t *testing.T, fixture Fixture, results Results) {
	wantOutput := injectVars(fixture.Expect.CliOutput, fixture.Endpoint, fixture.TlsData)
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
func checkStatusData(t *testing.T, fixture Fixture, results Results) {
	// check the env
	injectMapVars(fixture.Endpoint, fixture.Expect.Env, fixture.TlsData)
	if diff := cmp.Diff(fixture.Expect.Env, results.Env); diff != "" {
		t.Errorf("env data did not match fixture in %q (-want +got):\n%s", fixture.Name, diff)
	}

	// check diagnostics, use string maps so the diff output is easy to compare to json
	wantDiag := fixture.Expect.Diagnostics.ToStringMap()
	gotDiag := results.Diagnostics.ToStringMap()
	injectMapVars(fixture.Endpoint, wantDiag, fixture.TlsData)
	// there's almost always going to be cli_args in practice, so if the fixture has
	// an empty string, just ignore that key
	if wantDiag["cli_args"] == "" {
		gotDiag["cli_args"] = ""
	}
	for k, v := range wantDiag {
		if v == "*" {
			wantDiag[k] = gotDiag[k]
		}
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
	injectMapVars(fixture.Endpoint, wantConf, fixture.TlsData)
	if diff := cmp.Diff(wantConf, gotConf); diff != "" {
		t.Errorf("[%s] config data did not match fixture (-want +got):\n%s", fixture.Name, diff)
	}
}

// checkSpanData compares the data in spans received from otel-cli against the
// fixture data.
func checkSpanData(t *testing.T, fixture Fixture, results Results) {
	// check the expected span data against what was received by the OTLP server
	gotSpan := otlpclient.SpanToStringMap(results.Span, results.ResourceSpans)
	injectMapVars(fixture.Endpoint, gotSpan, fixture.TlsData)
	wantSpan := map[string]string{} // to be passed to cmp.Diff

	// verify all fields that were expected were present in output span
	for what := range fixture.Expect.SpanData {
		if _, ok := gotSpan[what]; !ok {
			t.Errorf("[%s] expected span to have key %q but it is not present", fixture.Name, what)
		}
	}

	// spanRegexChecks is a map of field names and regexes for basic presence
	// and validity checking of span data fields with expected set to "*"
	spanRegexChecks := map[string]*regexp.Regexp{
		"trace_id":   regexp.MustCompile(`^[0-9a-fA-F]{32}$`),
		"span_id":    regexp.MustCompile(`^[0-9a-fA-F]{16}$`),
		"name":       regexp.MustCompile(`^\w+$`),
		"parent":     regexp.MustCompile(`^[0-9a-fA-F]{32}$`),
		"kind":       regexp.MustCompile(`^\w+$`), // TODO: can validate more here
		"start":      regexp.MustCompile(`^\d+$`),
		"end":        regexp.MustCompile(`^\d+$`),
		"attributes": regexp.MustCompile(`\w+=.+`), // not great but should validate at least one k=v
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
			if wantVal == "*" {
				// * means if the above RE returns cleanly then pass the test
				if re.MatchString(gotVal) {
					delete(gotSpan, what)
					delete(wantSpan, what)
				} else {
					t.Errorf("[%s] span value %q for key %s is not valid", fixture.Name, gotVal, what)
				}
			}
		}
	}

	injectMapVars(fixture.Endpoint, wantSpan, fixture.TlsData)

	// a regular expression can be put in e.g. /^foo$/ to get evaluated as RE
	for key, wantVal := range wantSpan {
		if strings.HasPrefix(wantVal, "/") && strings.HasSuffix(wantVal, "/") {
			re := regexp.MustCompile(wantVal[1 : len(wantVal)-1]) // slice strips the /'s off
			if !re.MatchString(gotSpan[key]) {
				t.Errorf("regular expression %q did not match on %q", wantVal, gotSpan[key])
			}
			delete(gotSpan, key) // delete from both maps so cmp.Diff ignores them
			delete(wantSpan, key)
		}
	}

	// do a diff on a generated map that sets values to * when the * check succeeded
	if diff := cmp.Diff(wantSpan, gotSpan); diff != "" {
		t.Errorf("[%s] otel span info did not match fixture (-want +got):\n%s", fixture.Name, diff)
	}
}

// checkHeaders compares the expected and received headers.
func checkHeaders(t *testing.T, fixture Fixture, results Results) {
	// gzip encoding makes automatically comparing values tricky, so ignore it
	// unless the test actually specifies a length
	if _, ok := fixture.Expect.Headers["Content-Length"]; !ok {
		delete(results.Headers, "Content-Length")
	}

	injectMapVars(fixture.Endpoint, fixture.Expect.Headers, fixture.TlsData)
	injectMapVars(fixture.Endpoint, results.Headers, fixture.TlsData)

	for k, v := range fixture.Expect.Headers {
		if v == "*" {
			// overwrite value so cmp.Diff will ignore
			results.Headers[k] = "*"
		}
	}

	if diff := cmp.Diff(fixture.Expect.Headers, results.Headers); diff != "" {
		t.Errorf("[%s] headers did not match expected (-want +got):\n%s", fixture.Name, diff)
	}
}

// checkServerMeta compares the expected and received server metadata.
func checkServerMeta(t *testing.T, fixture Fixture, results Results) {
	injectMapVars(fixture.Endpoint, fixture.Expect.ServerMeta, fixture.TlsData)
	injectMapVars(fixture.Endpoint, results.ServerMeta, fixture.TlsData)

	if diff := cmp.Diff(fixture.Expect.ServerMeta, results.ServerMeta); diff != "" {
		t.Errorf("[%s] server metadata did not match expected (-want +got):\n%s", fixture.Name, diff)
	}
}

// checkFuncs runs through the list of funcs in the fixture to do
// complex checking on data. Funcs should call t.Errorf, etc. as needed.
func checkFuncs(t *testing.T, fixture Fixture, results Results) {
	for _, f := range fixture.CheckFuncs {
		f(t, fixture, results)
	}
}

// runOtelCli runs the a server and otel-cli together and captures their
// output as data to return for further testing.
func runOtelCli(t *testing.T, fixture Fixture) (string, Results) {
	started := time.Now()

	results := Results{
		SpanData:   map[string]string{},
		Env:        map[string]string{},
		SpanEvents: []*tracepb.Span_Event{},
	}

	// these channels need to be buffered or the callback will hang trying to send
	rcvSpan := make(chan *tracepb.Span, 100) // 100 spans is enough for anybody
	rcvEvents := make(chan []*tracepb.Span_Event, 100)

	// otlpserver calls this function for each span received
	cb := func(ctx context.Context, span *tracepb.Span, events []*tracepb.Span_Event, rss *tracepb.ResourceSpans, headers map[string]string, meta map[string]string) bool {
		rcvSpan <- span
		rcvEvents <- events

		results.ServerMeta = meta
		results.ResourceSpans = rss
		results.SpanCount++
		results.EventCount += len(events)
		results.Headers = headers

		// true tells the server we're done and it can exit its loop
		return results.SpanCount >= fixture.Expect.SpanCount
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
		tlsConf := fixture.TlsData.serverTLSConf.Clone()
		if fixture.Config.ServerTLSAuthEnabled {
			tlsConf.ClientAuth = tls.RequireAndVerifyClientCert
		}
		listener, err = tls.Listen("tcp", "localhost:0", tlsConf)
	} else {
		listener, err = net.Listen("tcp", "localhost:0")
	}
	endpoint := listener.Addr().String()
	if err != nil {
		// t.Fatalf is not allowed since we run this in a goroutine
		t.Errorf("[%s] failed to listen on OTLP endpoint %q: %s", fixture.Name, endpoint, err)
		return endpoint, results
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
			args = append(args, injectVars(v, endpoint, fixture.TlsData))
		}
	}
	statusCmd := exec.Command("./otel-cli", args...)
	statusCmd.Env = mkEnviron(endpoint, fixture.Config.Env, fixture.TlsData)

	// have command write output into string buffers
	var cliOut bytes.Buffer
	statusCmd.Stdout = &cliOut
	statusCmd.Stderr = &cliOut

	err = statusCmd.Start()
	if err != nil {
		t.Fatalf("[%s] error starting otel-cli: %s", fixture.Name, err)
	}

	stopKiller := make(chan struct{}, 1)
	if fixture.Config.KillAfter != 0 {
		go func() {
			select {
			case <-time.After(fixture.Config.KillAfter):
				err := statusCmd.Process.Signal(fixture.Config.KillSignal)
				if err != nil {
					log.Fatalf("[%s] error sending signal %s to pid %d: %s", fixture.Name, fixture.Config.KillSignal, statusCmd.Process.Pid, err)
				}
			case <-stopKiller:
				return
			}
		}()
	} else {
		go func() {
			select {
			case <-time.After(serverTimeout):
				t.Logf("[%s] timeout, killing process...", fixture.Name)
				results.TimedOut = true
				err = statusCmd.Process.Kill()
				if err != nil {
					// TODO: this might be a bit fragile, soften this up later if it ends up problematic
					log.Fatalf("[%s] %d timeout process kill failed: %s", fixture.Name, serverTimeout, err)
				}
			case <-stopKiller:
				return
			}
		}()
	}

	// grab stderr & stdout comingled so that if otel-cli prints anything to either it's not
	// supposed to it will cause e.g. status json parsing and other tests to fail
	t.Logf("[%s] going to exec 'env -i %s %s'", fixture.Name, strings.Join(statusCmd.Env, " "), strings.Join(statusCmd.Args, " "))
	err = statusCmd.Wait()

	results.CliOutput = cliOut.String()
	results.ExitCode = statusCmd.ProcessState.ExitCode()
	results.CommandFailed = !statusCmd.ProcessState.Exited()
	if err != nil {
		t.Logf("[%s] command exited: %s", fixture.Name, err)
	}

	// send stop signals to the timeouts and OTLP server
	cancelServerTimeout <- struct{}{}
	stopKiller <- struct{}{}
	cs.Stop()

	// only try to parse status json if it was a status command
	// TODO: support variations on otel-cli where status isn't the first arg
	if len(args) > 0 && args[0] == "status" && results.ExitCode == 0 {
		err = json.Unmarshal(cliOut.Bytes(), &results)
		if err != nil {
			t.Errorf("[%s] parsing otel-cli status output failed: %s", fixture.Name, err)
			t.Logf("[%s] output received: %q", fixture.Name, cliOut)
			return endpoint, results
		}

		// remove PATH from the output but only if it's exactly what we set on exec
		if path, ok := results.Env["PATH"]; ok {
			if path == minimumPath {
				delete(results.Env, "PATH")
			}
		}
	}

	// when no spans are expected, return without reading from the channels
	if fixture.Expect.SpanCount == 0 {
		return endpoint, results
	}

	// grab the spans & events from the server off the channels it writes to
	remainingTimeout := serverTimeout - time.Since(started)
	var gatheredSpans int
gather:
	for {
		select {
		case <-time.After(remainingTimeout):
			break gather
		case results.Span = <-rcvSpan:
			// events is always populated at the same time as the span is sent
			// and will always send at least an empty list
			results.SpanEvents = <-rcvEvents

			// with this approach, any mismatch in spans produced and expected results
			// in a timeout with the above time.After
			gatheredSpans++
			if gatheredSpans == results.SpanCount {
				// TODO: it would be slightly nicer to use plural.Selectf instead of 'span(s)'
				t.Logf("[%s] test gathered %d span(s)", fixture.Name, gatheredSpans)
				break gather
			}
		}
	}

	return endpoint, results
}

// mkEnviron converts a string map to a list of k=v strings and tacks on PATH.
func mkEnviron(endpoint string, env map[string]string, tlsData TlsSettings) []string {
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
func injectMapVars(endpoint string, target map[string]string, tlsData TlsSettings) {
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
func injectVars(in, endpoint string, tlsData TlsSettings) string {
	out := strings.ReplaceAll(in, "{{endpoint}}", endpoint)
	out = strings.ReplaceAll(out, "{{tls_ca_cert}}", tlsData.caFile)
	out = strings.ReplaceAll(out, "{{tls_client_cert}}", tlsData.clientFile)
	out = strings.ReplaceAll(out, "{{tls_client_key}}", tlsData.clientPrivKeyFile)
	return out
}
