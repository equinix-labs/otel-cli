## Synopsis

otel-cli is a command-line tool for sending OpenTelemetry events. It is written in
Go and intended to be used in shell scripts and other places where the best option
available for sending events is executing another program.

Since this needs to connect to the OTLP endpoint on each run, it is highly recommended
to have a localhost opentelemetry collector running so this doesn't slow down your
program too much and you don't spam outbound connections on each command.

Examples:
```shell
# run a program inside a span
otel-cli exec --service-name my-service --span-name "curl google" curl https://google.com

# otel-cli propagates span parents via envvars so you can chain it to create spans
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
```

Note: just made-up examples for now for me to think through the CLI parameters

```shell
# send all events to the local OpenTelemetry OTLP endpoint and it will
# forward to whatever is downstream, this also avoids spamming some external
# server for connections in your shell scripts
# (not implemented yet)
export OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:55681

# (maybe bad) idea - persist context to a tempfile across executions
otel_context=$(mktemp)
otel-cli --context $otel_context --stdout ...

# but probably a simple way to slurp some json or yaml or just key/value data
# would make labeling more efficient for some folks?
cat > labels.json <<EOJSON
{
  "someKey": "someValue"
}
EOJSON
otel-cli --json-labels labels.json

# silly idea...
# not a reimplementation of curl, just smart enough to pass through params
# and add the traceparent header automatically, maybe get curl into a mode
# where more data can be pulled out of the request?
otel-cli curl <regular curl options>
# maybe adds "-o 'SendEnv TRACEPARENT'" to the command?
otel-cli ssh <regular ssh options>
```

## Easy local dev

First, this needs some work to be good. Once the config plumbing is in
place this can hopefully stop using `--net host`

Run opentelemetry collector locally in debug mode in one window, and
hack on otel-cli in another..

If you have a Honeycomb API key and want to forward your data there,
put the API key in HONEYCOMB_TEAM and set HONEYCOMB_DATASET to the
dataset, e.g. `playground`.

```shell
export HONEYCOMB_TEAM= # put your api key here
export HONEYCOMB_DATASET=playground

docker run --name otel-collector --net host \
   --env HONEYCOMB_TEAM \
   --env HONEYCOMB_DATASET \
	--volume $(pwd)/local/otel-local-config.yaml:/local.yaml \
	public.ecr.aws/aws-observability/aws-otel-collector:latest \
		--config /local.yaml
```

Then it should just work to run otel-cli:

```shell
go run . span --options...
```

## Ideas

   * a --spanContext file path or similar for persisting span context across executions
   * add some shell examples for:
      * using bash trap(1p) to send events

## License

Proprietary

BUT: I'm writing this so it can be open sourced later and do wish to avoid making anything
Equinix or observability provider specific.
