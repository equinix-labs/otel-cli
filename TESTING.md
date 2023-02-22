# Testing otel-cli

## Synopsis

otel-cli's primary method of testing is functional, implemented in
`main_test.go` and accompanying files. It sets up a server and runs an otel-cli
build through a number of tests to verify that everything from environment
variable passing to server TLS negotiation work as expected.

## Unit Testing

Do it. It doesn't have to be fancy, just exercise the code a little. It's more
about all of us being able to iterate quickly than reaching total coverage.

Most unit tests are in the `otelcli` package. The tests in the root of this
project are not unit tests.

## The otel-cli Test Harness

When `go test` is run in the root of this project, it runs the available
`./otel-cli` binary through a suite of tests, providing otel-cli with its
endpoint information (via templates) and examining the payloads received on the
server.

The otel-cli test harness is more complex than otel-cli itself. Its goal is to
be able to test that setting e.g. `OTEL_EXPORTER_OTLP_CLIENT_KEY` works all the
way through to authenticating to a TLS server. The bugs are going to exist in
the glue code, since that's mostly what otel-cli is. Each of Cobra,
`encoding/json`, and opentelemetry-go are thorougly unit and battle tested. So,
otel-cli tests a build in a functional test harness.

Tests are defined in `data_for_test.go` in Go data structures. Suites are a a
group of Fixtures that go together. Mostly Suites are necessary for the
backgrounding feature, to test e.g. `otel-cli span background`, and to organize
tests by functionality, etc.. Fixtures configure everything for an otel-cli
invocation, and what to expect back from it.

The OTLP server functionality originally built for `otel-cli server tui` is
re-used in the tests to run a server in a goroutine. It supports both gRPC and
HTTP variants of OTLP, and can be configured with TLS. This allows otel-cli to
connect to a server and send traces, which the harness then compares to
expectations defined in the test Fixture.

otel-cli has a special subcommand, `otel-cli status` that sends a span and
reports back on otel-cli's internal state. The test harness detects status
commands and can check data in it.

`tls_for_test.go` implements an ephemeral certificate authority that is created
and destroyed on each run. The rest of the test harness injects the CA and certs
created into the tests, allowing for full-system testing.

A Fixture can be configured to run "in the background". In this case, the harness
will behave as if you ran the command `./otel-cli &` and let following fixtures
run on top of it. This is mostly used to test `otel-cli span background`, which
exists primarily to run as a backgrounded job in a shell script. When background
jobs are in use, be careful with test names as they are used as a key to manage
the process.

## Adding Tests

For a new area of functionality, you'll want to add a Suite. A Suite is mostly
for organization of tests, but is also used to manage state when testing background
jobs. A Fixture is made of two parts: an otel-cli command configuration, and a
data structure of expected results. The harness presents otel-cli with the exact
ARGV specified in `Config.CliArgs`, and a clean environment with only the envvars
provided in the `Env` stringmap. The output from otel-cli is captured with stdout
and stderr combined. This can be tested against as well.

When otel-cli reads an environment variable it clears it, to prevent opentelemetry-go
from double-processing it. This is why envvars set in `Config.Env` don't show up
in `Results.Env`.

It is often wise to pass `"--fail", "--verbose"` to CliArgs for debugging and it's
fine to leave them on permanently. Without them otel-cli will be silent about
failures and you'll get a confusing test output.

Most of the time it's best to copy an existing Suite or Fixture and adjust it to
the case you're trying to test. Please try to clean out any unneeded config when
you do this so the tests are easy to understand. It's not bad to to test a little
extra surface area, just try to keep things readable.
