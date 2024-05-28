#!/bin/bash
# an otel-cli demo of nested exec
#
# this isn't necessarily practical, but it demonstrates how the TRACEPARENT
# environment variable carries the context from invocation to invocation
# so that the tracing provider (e.g. Honeycomb) can put it all back together

# set the service name automatically on calls to otel-cli
export OTEL_SERVICE_NAME="otel-cli-demo"

# generate a new trace & span, cli will print out the 'export TRACEPARENT'
carrier=$(mktemp)
../otel-cli span -n "traceparent demo $0" --tp-print --tp-carrier $carrier

# this will start a child span, and run another otel-cli as its program
../otel-cli exec \
	--name       "hammer the server for sweet sweet data" \
	--kind       "client" \
	--tp-carrier $carrier \
	--verbose \
     	--fail \
	-- \
	../otel-cli exec -n fake-server -k server /bin/echo 500 NOPE
	# ^ child span, the responding "server" that just echos NOPE

