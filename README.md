# otel-cli

[![](https://img.shields.io/badge/stability-experimental-lightgrey.svg)](https://github.com/packethost/standards/blob/master/experimental-statement.md)

otel-cli is a command-line tool for sending OpenTelemetry traces. It is written in
Go and intended to be used in shell scripts and other places where the best option
available for sending spans is executing another program.

otel-cli can be added to your scripts with no configuration and it will run as normal
but in non-recording mode and will emit no traces. This follows the OpenTelemetry community's
philosophy of "first, do no harm" and makes it so you can add otel-cli to your code and
later turn it on.

Since otel-cli needs to connect to the OTLP endpoint on each run, it is highly recommended
to use a localhost opentelemetry collector that can buffer spans so that the connection
cost does not slow down your program too much.

## Getting Started

We publish a number of package formats for otel-cli, including tar.gz, zip (windows),
apk (Alpine), rpm (Red Hat variants), deb (Debian variants), and a brew tap. These
can be found on the repo's [Releases](https://github.com/equinix-labs/otel-cli/releases) page.

On most platforms the easiest way is a go get:

```shell
go install github.com/equinix-labs/otel-cli@latest
```

Docker images are published for each otel-cli release as well:

```shell
docker pull ghcr.io/equinix-labs/otel-cli:latest
docker run ghcr.io/equinix-labs/otel-cli:latest status
```

To use the brew tap e.g. on MacOS:

```shell
brew tap equinix-labs/otel-cli
brew install otel-cli
```

Alternatively, clone the repo and build it locally:

```shell
git clone git@github.com:equinix-labs/otel-cli.git
cd otel-cli
go build
```

## Examples

```shell
# run otel-cli as a local OTLP server and print traces to your console
# run this in its own terminal and try some of the commands below!
otel-cli server tui

# configure otel-cli to talk the the local server spawned above
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317

# run a program inside a span
otel-cli exec --service my-service --name "curl google" curl https://google.com

# otel-cli propagates context via envvars so you can chain it to create child spans
otel-cli exec --kind producer "otel-cli exec --kind consumer sleep 1"

# if a traceparent envvar is set it will be automatically picked up and
# used by span and exec. use --tp-ignore-env to ignore it even when present
export TRACEPARENT=00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01

# you can pass the traceparent to a child via arguments as well
# {{traceparent}} in any of the command's arguments will be replaced with the traceparent string
otel-cli exec --name "curl api" -- \
   curl -H 'traceparent: {{traceparent}}' https://myapi.com/v1/coolstuff

# create a span with a custom start/end time using either RFC3339,
# same with the nanosecond extension, or Unix epoch, with/without nanos
otel-cli span --start 2021-03-24T07:28:05.12345Z --end 2021-03-24T07:30:08.0001Z
otel-cli span --start 1616620946 --end 1616620950.241980634
# so you can do this:
start=$(date --rfc-3339=ns) # rfc3339 with nanoseconds
some-interesting-program --with-some-options
end=$(date +%s.%N) # Unix epoch with nanoseconds
otel-cli span -n my-script -s some-interesting-program --start $start --end $end

# for advanced cases you can start a span in the background, and
# add events to it, finally closing it later in your script
sockdir=$(mktemp -d)
otel-cli span background \
   --service $0          \
   --name "$0 runtime"   \
   --sockdir $sockdir & # the & is important here, background server will block
sleep 0.1 # give the background server just a few ms to start up
otel-cli span event --name "cool thing" --attrs "foo=bar" --sockdir $sockdir
otel-cli span end --sockdir $sockdir
# or you can kill the background process and it will end the span cleanly
kill %1

# server mode can also write traces to the filesystem, e.g. for testing
dir=$(mktemp -d)
otel-cli server json --dir $dir --timeout 60 --max-spans 5
```

## Configuration

Everything is configurable via CLI arguments, json config, and environment
variables. If no endpoint is specified, otel-cli will run in non-recording
mode and not attempt to contact any servers.

All three modes of config can be mixed. Command line args are loaded first,
then config file, then environment variables.

| CLI argument         | environment variable                  | config file key          | example value  |
| -------------------- | ------------------------------------- | ------------------------ | -------------- |
| --endpoint           | OTEL_EXPORTER_OTLP_ENDPOINT           | endpoint                 | localhost:4317       |
| --traces-endpoint    | OTEL_EXPORTER_OTLP_TRACES_ENDPOINT    | traces_endpoint          | https://localhost:4318/v1/traces |
| --protocol           | OTEL_EXPORTER_OTLP_PROTOCOL           | protocol                 | http/protobuf  |
| --insecure           | OTEL_EXPORTER_OTLP_INSECURE           | insecure                 | false          |
| --timeout            | OTEL_EXPORTER_OTLP_TIMEOUT            | timeout                  | 1s             |
| --otlp-headers       | OTEL_EXPORTER_OTLP_HEADERS            | otlp_headers             | k=v,a=b        |
| --otlp-blocking      | OTEL_EXPORTER_OTLP_BLOCKING           | otlp_blocking            | false          |
| --config             | OTEL_CLI_CONFIG_FILE                  | config_file              | config.json    |
| --verbose            | OTEL_CLI_VERBOSE                      | verbose                  | false          |
| --fail               | OTEL_CLI_FAIL                         | fail                     | false          |
| --service            | OTEL_SERVICE_NAME                     | service_name             | myapp          |
| --kind               | OTEL_CLI_TRACE_KIND                   | span_kind                | server         |
| --status-code        | OTEL_CLI_STATUS_CODE                  | span_status_code         | error          |
| --status-description | OTEL_CLI_STATUS_DESCRIPTION           | span_status_description  | cancelled      |
| --attrs              | OTEL_CLI_ATTRIBUTES                   | span_attributes          | k=v,a=b        |
| --force-trace-id     | OTEL_CLI_FORCE_TRACE_ID               | force_trace_id           | 00112233445566778899aabbccddeeff |
| --force-span-id      | OTEL_CLI_FORCE_SPAN_ID                | force_span_id            | beefcafefacedead |
| --force-parent-span-id | OTEL_CLI_FORCE_PARENT_SPAN_ID       | force_parent_span_id     | eeeeeeb33fc4f3d3 |
| --tp-required        | OTEL_CLI_TRACEPARENT_REQUIRED         | traceparent_required     | false          |
| --tp-carrier         | OTEL_CLI_CARRIER_FILE                 | traceparent_carrier_file | filename.txt   |
| --tp-ignore-env      | OTEL_CLI_IGNORE_ENV                   | traceparent_ignore_env   | false          |
| --tp-print           | OTEL_CLI_PRINT_TRACEPARENT            | traceparent_print        | false          |
| --tp-export          | OTEL_CLI_EXPORT_TRACEPARENT           | traceparent_print_export | false          |
| --tls-no-verify      | OTEL_CLI_TLS_NO_VERIFY                | tls_no_verify    | false                  |
| --tls-ca-cert        | OTEL_EXPORTER_OTLP_CERTIFICATE        | tls_ca_cert      | /ca/ca.pem             |
| --tls-client-key     | OTEL_EXPORTER_OTLP_CLIENT_KEY         | tls_client_key   | /keys/client-key.pem   |
| --tls-client-cert    | OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE | tls_client_cert  | /keys/client-cert.pem  |

[Valid timeout units](https://pkg.go.dev/time#ParseDuration) are "ns", "us"/"µs", "ms", "s", "m", "h".

### Endpoint URIs

otel-cli deviates from the OTel specification for endpoint URIs. Mainly, otel-cli supports
bare host:port for grpc endpoints and continues to default to gRPC. The optional http/json
is not supported by opentelemetry-go so otel-cli does not support it. To use gRPC with an
http endpoint, set the protocol with --protocol or the envvar.

   * bare `host:port` endpoints are assumed to be gRPC and are not supported for HTTP
   * `http://` and `https://` are assumed to be HTTP unless --protocol is set to `grpc`.
   * loopback addresses without an https:// prefix are assumed to be unencrypted

### Header and Attribute formatting

Headers and attributes allow for `key=value,k=v` style formatting. Internally both
otel-cli and pflag use Go's `encoding/csv` to parse these values. Therefore, if you want
to pass commas in a value, follow CSV quoting rules and quote the whole k=v pair.
Double quotes need to be escaped so the shell doesn't interpolate them. Once that's done,
embedding commas will work fine.

```shell
otel-cli span --attrs item1=value1,\"item2=value2,value3\",item3=value4
otel-cli span --attrs 'item1=value1,"item2=value2,value3",item3=value4'
```

### Docker TLS Certificates

As of release 0.4.2, otel-cli containers are built off the latest Alpine base
image which contains the base CA certificate bundles. In order to override
these for e.g. a self-signed certificate, the best bet is to volume mount your
own /etc/ssl into the container, and it should get picked up by otel-cli and Go's
TLS libraries.

```shell
docker run -v /etc/ssl:/etc/ssl ghcr.io/equinix-labs/otel-cli:latest status
```

## Easy local dev

We want working on otel-cli to be easy, so we've provided a few different ways to get
started. In general, there are three things you need:

- A working Go environment
- A built (or installed) copy of otel-cli itself
- A system to receive/inspect the traces you generate

### 1. A working Go environment

Providing instructions on getting Go up and running on your machine is out of scope for this
README. However, the good news is that it's fairly easy to do! You can follow the normal
[Installation instructions](https://golang.org/doc/install) from the Go project itself.

### 2. A built (or installed) copy of otel-cli itself

If you're planning on making changes to otel-cli, we recommend building the project locally: `go build`

But, if you just want to quickly try out otel-cli, you can also just install it directly: `go get github.com/equinix-labs/otel-cli`. This will place the command in your `GOPATH`. If your `GOPATH` is in your `PATH` you should be all set.

### 3. A system to receive/inspect the traces you generate

otel-cli can run as a server and accept OTLP connections. It has two modes, one prints to your console
while the other writes to JSON files.

```shell
otel-cli server tui
otel-cli server json --dir $dir --timeout 60 --max-spans 5
```

Many SaaS vendors accept OTLP these days so one option is to send directly to those. This is not
recommended for production since it will slow your code down on the roundtrips. It is recommended
to use an opentelemetry-collector locally.

Another option is to use [`otel-desktop-viewer`](https://github.com/CtrlSpice/otel-desktop-viewer). 
This will bring up a server that can accept OTLP connections.

If you're not sure what to choose, try `otel-cli server tui` or `otel-desktop-viewer`.

### `otel-desktop-viewer` setup

```shell
# install the CLI tool
go install github.com/CtrlSpice/otel-desktop-viewer@latest

# run it!
$(go env GOPATH)/bin/otel-desktop-viewer

# if you have $GOPATH/bin added to your $PATH you can call it directly!
otel-desktop-viewer

# if not you can add it to your $PATH by running this or adding it to
# your startup script (usually ~/.bashrc or ~/.zshrc)
export PATH="$(go env GOPATH)/bin:$PATH"
```

The OpenTelemetry collector is listening on `localhost:4318`, and the UI will be running on
`localhost:8000`.

```shell
# start the desktop viewer (best to do this in a separate terminal)
otel-desktop-viewer

# configure otel-cli to send to our desktop viewer endpoint
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318

# use otel-cli to generate spans!
otel-cli exec --service my-service --name "curl google" curl https://google.com
```

This trace will be available at `localhost:8000`.

### SaaS tracing vendor

We've provided Honeycomb, LightStep, and Elastic configurations that you could also use,
if you're using one of those vendors today. It's still pretty easy to get started:

```shell
# optional: to send data to an an OTLP-enabled tracing vendor, pass in your
# API auth token over an environment variable and modify
# `local/otel-vendor-config.yaml` according to the comments inside
export LIGHTSTEP_TOKEN= # Lightstep API key (otlp/1 in the yaml)
export HONEYCOMB_TEAM=  # Honeycomb API key (otlp/2 in the yaml)
export HONEYCOMB_DATASET=playground # Honeycomb dataset
export ELASTIC_TOKEN= # Elastic token for the APM server.

docker run \
   --env LIGHTSTEP_TOKEN \
   --env HONEYCOMB_TEAM \
   --env HONEYCOMB_DATASET \
   --env ELASTIC_TOKEN \
   --name otel-collector \
   --net host \
   --volume $(pwd)/local/otel-vendor-config.yaml:/local.yaml \
   public.ecr.aws/aws-observability/aws-otel-collector:latest \
      --config /local.yaml
```

Then it should just work to run otel-cli:

```shell
./otel-cli span -n "testing" -s "my first test span"
# or for quick iterations:
go run . span -n "testing" -s "my first test span"
```

## Contributing

Please file issues and PRs on the GitHub project at https://github.com/equinix-labs/otel-cli

## Releases

Releases are managed by goreleaser. Currently this is limited to @tobert due to rules in
the equinix-labs organization. For now releases are not automated, but will be by the time
a v1.0 rolls out and the test suite is robust enough that we feel confident.

Testing the release: `goreleaser release --snapshot --rm-dist`

To release, a GitHub personal access token is required. The release also needs to be tagged
in git.

```shell
docker login ghcr.io # log into GitHub Docker repo
gh repo list         # make sure GitHub PAT is working
git checkout main    # release tags must be off the main branch
git pull --rebase    # get the latest HEAD
git tag v0.1.1       # tag HEAD with the next version
git push --tags      # push new tag up to GitHub
goreleaser release --rm-dist
```

## License

Apache 2.0, see LICENSE

