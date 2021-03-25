#!/bin/bash
# an otel-cli demo of nested exec
#
# this isn't necessarily practical, but it demonstrates how the TRACEPARENT
# environment variable carries the context from invocation to invocation
# so that the tracing provider (e.g. Honeycomb) can put it all back together

# generate a new trace & span, cli will print out the TRACEPARENT
carrier=$(mktemp)
../otel-cli span -s $0 -n "traceparent demo" -p |tee $carrier
source $carrier    # sets TRACEPARENT (todo - this is not entirely safe)
export TRACEPARENT # make it visible to child processes

# this will start a child span, and run another otel-cli as its program
../otel-cli exec \
	--service-name "fake-client" \
	--span-name    "hammer the server for sweet sweet data" \
	--kind         "client" \
	"../otel-cli exec -n fake-server -s 'put up with the clients nonsense' -k server echo 500 NOPE"
	# ^ child span, the responding "server" that just echos NOPE

