#!/bin/bash
# an otel-cli demo of span events

t=$(date +%s)
st=$(($t - 1))
et=$(($st + 2))


# generate a new trace & span to attach events to
carrier=$(mktemp)
../otel-cli span -s $0 -n "span event demo" -p --start $st --end $et > $carrier
. $carrier

echo "TRACEPARENT=$TRACEPARENT"

../otel-cli span event \
	--event-name "interesting thing!" \
	--print-tp

