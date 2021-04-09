#!/bin/bash
# an otel-cli demo of span background
#
# This demo shows span background functionality with events added to the span
# while it's running in the background, then a child span is created and
# the background span is ended gracefully.

set -e
set -x

carrier=$(mktemp)    # traceparent propagation via tempfile
sockdir=$(mktemp -d) # a unix socket will be created here

# start the span background server, set up trace propagation, and
# time out after 10 seconds (which shouldn't be reached)
../otel-cli span background \
    --tp-carrier $carrier \
    --sockdir $sockdir \
    --tp-print \
    --service $0 \
    --name "$0 script execution" \
    --timeout 10 &

data1=$(uuidgen)

# add an event to the span running in the background, with an attribute
# set to the uuid we just generated
../otel-cli span event \
    --name "did a thing" \
    --sockdir $sockdir \
    --attrs "data1=$data1"

# waste some time
sleep 1

# add an event that says we wasted some time
../otel-cli span event --name "slept 1 second" --sockdir $sockdir

# run a shorter sleep inside a child span, also note that this is using
# --tp-required so this will fail loudly if there is no traceparent
# available
../otel-cli exec \
    --service $0 \
    --name "sleep 0.2" \
    --tp-required \
    --tp-carrier $carrier \
    --tp-print \
    sleep 0.2

# finally, tell the background server we're all done and it can exit
../otel-cli span end --sockdir $sockdir

