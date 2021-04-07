# otel-cli

[![](https://img.shields.io/badge/stability-experimental-lightgrey.svg)](https://github.com/packethost/standards/blob/master/experimental-statement.md)

otel-cli is a command-line tool for sending OpenTelemetry traces. It is written in
Go and intended to be used in shell scripts and other places where the best option
available for sending spans is executing another program.

Since this needs to connect to the OTLP endpoint on each run, it is highly recommended
to have a localhost opentelemetry collector running so this doesn't slow down your
program too much and you don't spam outbound connections on each command.

## Getting Started

The easiest way is a go get:

```shell
go get github.com/packethost/otel-cli
```

Alternatively, clone the repo and build it locally:

```shell
git clone git@github.com:packethost/otel-cli.git
cd otel-cli
go build
```

## Examples

```shell
# run a program inside a span
otel-cli exec --service my-service --name "curl google" curl https://google.com

# otel-cli propagates context via envvars so you can chain it to create child spans
otel-cli exec --kind producer "otel-cli exec --kind consumer sleep 1"

# if a traceparent envvar is set it will be automatically picked up and
# used by span and exec. use --ignore-tp-env to ignore it even when present
export TRACEPARENT=00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01

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
```

## Configuration

Everything is configurable via CLI arguments, and many of those arguments can
also be configured via file or environment variables.

| CLI argument    | environment variable          | config file key | example value  |
| --------------- | ----------------------------- | --------------- | -------------- |
| --endpoint      | OTEL_EXPORTER_OTLP_ENDPOINT   | endpoint        | localhost:4317 |
| --insecure      | OTEL_EXPORTER_OTLP_INSECURE   | insecure        | false          |
| --otlp-headers  | OTEL_EXPORTER_OTLP_HEADERS    | otlp-headers    | key=value      |
| --otlp-blocking | OTEL_EXPORTER_OTLP_BLOCKING   | otlp-blocking   | false          |
| --service       | OTEL_CLI_SERVICE_NAME         | service         | myapp          |
| --kind          | OTEL_CLI_TRACE_KIND           | kind            | server         |
| --attrs         | OTEL_CLI_ATTRIBUTES           | attrs           | k=v,a=b        |
| --tp-required   | OTEL_CLI_TRACEPARENT_REQUIRED | tp-required     | false          |
| --tp-carrier    | OTEL_CLI_CARRIER_FILE         | tp-carrier      | filename.txt   |
| --tp-ignore-env | OTEL_CLI_IGNORE_ENV           | tp-ignore-env   | false          |
| --tp-print      | OTEL_CLI_PRINT_TRACEPARENT    | tp-print        | false          |
| --tp-export     | OTEL_CLI_EXPORT_TRACEPARENT   | tp-export       | false          |

## Easy local dev

First, this needs some work to be good. Once the config plumbing is in
place this can hopefully stop using `--net host`

Run opentelemetry collector locally in debug mode in one window, and
hack on otel-cli in another..

If you have a Honeycomb API key and want to forward your data there,
put the API key in HONEYCOMB_TEAM and set HONEYCOMB_DATASET to the
dataset, e.g. `playground`.

```shell
# optional: to send data to an an OTLP-enabled tracing vendor, pass in your
# API auth token over an environment variable and modify
# `local/otel-local-config.yaml` according to the comments inside
export LIGHTSTEP_TOKEN= # Lightstep API key (otlp/1 in the yaml)
export HONEYCOMB_TEAM=  # Honeycomb API key (otlp/2 in the yaml)
export HONEYCOMB_DATASET=playground # Honeycomb dataset

docker run \
   --env LIGHTSTEP_TOKEN \
   --env HONEYCOMB_TEAM \
   --env HONEYCOMB_DATASET \
   --name otel-collector \
   --net host \
   --volume $(pwd)/local/otel-local-config.yaml:/local.yaml \
   public.ecr.aws/aws-observability/aws-otel-collector:latest \
      --config /local.yaml
```

Then it should just work to run otel-cli:

```shell
go build
./otel-cli span -n "testing" -s "my first test span"
# or for quick iterations:
go run . span -n "testing" -s "my first test span"
```

## Ideas

   * add some shell examples for:
      * using bash trap(1p) to send events
   * examples for connecting collector to other vendors' OTLP endpoints
   * span background doodles: https://gist.github.com/tobert/ceb2cd9b18ab7ab09e1ea7e3bf150d9d

## Contributing

Please file issues and PRs on the GitHub project at https://github.com/packethost/otel-cli

## Releases

This project is really new and still experiemental, releases are TBD.

## License

Apache 2.0, see LICENSE

