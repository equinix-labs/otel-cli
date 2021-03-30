#!/bin/bash
# an otel-cli demo

# this isn't super precise because of process timing but good enough
# in many cases to be useful
st=$(date +%s.%N) # unix epoch time with nanoseconds
data1=$(uuidgen)
data2=$(uuidgen)
et=$(date +%s.%N)

# don't worry, there are also short options :)
../otel-cli span \
	--service-name "demo.sh"     \
	--span-name    "hello world" \
	--kind         "client"      \
	--start        $st           \
	--end          $et           \
	--print-tp                   \
	--attrs "my.data1=$data1,my.data2=$data2"

cat >/dev/null<<EOF
A command-line interface for generating OpenTelemetry data on the command line.

Usage:
  otel-cli [command]

Available Commands:
  exec        execute the command provided
  span        create an OpenTelemetry span and send it
  help        Help about any command

Flags:
  -a, --attrs stringToString   a comma-separated list of key=value attributes (default [])
  -c, --config string          config file (default is $HOME/.otel-cli.yaml)
      --ignore-tp-env          ignore the TRACEPARENT envvar even if it's set
  -k, --kind string            set the trace kind, e.g. internal, server, client, producer, consumer (default "client")
  -p, --print-span             print the trace id, span id, and the w3c-formatted traceparent representation of the new span
  -n, --service-name string    set the name of the application sent on the traces (default "otel-cli")
  -s, --span-name string       set the name of the application sent on the traces (default "todo-generate-default-span-names")
  -h, --help                   help for otel-cli

Use "otel-cli [command] --help" for more information about a command.
EOF

