#!/bin/bash
# an otel-cli demo of workarounds for race conditions on span background
#
# otel-cli span background is usually run as a subprocess with & in the command
# as below. An issue that shows up sometimes is a race condition where the shell
# starts otel-cli in the background, and then immediately calls otel-cli span
# or similar hoping to use the --tp-carrier file, which might not be created
# before the process looks for it. There are a couple solutions below.

set -e
set -x

carrier=$(mktemp)    # traceparent propagation via tempfile
sockdir=$(mktemp -d) # a unix socket will be created here

export OTEL_SERVICE_NAME="otel-cli-demo"

../otel-cli span background \
    --tp-carrier $carrier \
    --sockdir $sockdir \
    --service otel-cli \
    --name "$0 script execution #1" \
    --timeout 10 &

# On Linux, the inotifywait command will do the trick, waiting for the
# file to be written. Without a timeout it could hang forever if it loses
# the race and otel-cli finishes writing the carrier before inotifywait
# starts watching. A short timeout ensures it won't hang.
[ ! -e $carrier ] && inotifywait --timeout 0.1 $carrier
../otel-cli span --tp-carrier $carrier --name "child of span background, after inotifywait"
../otel-cli span end --sockdir $sockdir

# start another one for the second example
../otel-cli span background \
    --tp-carrier $carrier \
    --sockdir $sockdir \
    --service otel-cli \
    --name "$0 script execution #2" \
    --timeout 10 &

# otel-cli span event already waits for span background's socket file
# to appear so just sending an event will do enough synchronization, at
# the cost of a meaningless event.
../otel-cli span event --sockdir $sockdir --name "wait for span background"
../otel-cli span --tp-carrier $carrier --name "child of span background, after span event"
../otel-cli span end --sockdir $sockdir
