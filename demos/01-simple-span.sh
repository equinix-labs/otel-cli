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
	--service  "otel-cli-demo"    \
	--name     "hello world" \
	--kind     "client"      \
	--start    $st           \
	--end      $et           \
	--tp-print               \
	--attrs "my.data1=$data1,my.data2=$data2"
