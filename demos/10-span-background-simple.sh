#!/bin/bash
# an otel-cli demo of span background

../otel-cli span background \
    --service $0 \
    --name "executing $0" \
    --timeout 2 &
#               ^ run otel-cli in the background
sleep 1

# that's it, that's the demo
# when this script exits, otel-cli will exit too so total runtime will
# be a bit over 1 second
