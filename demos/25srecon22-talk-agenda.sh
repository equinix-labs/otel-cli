#!/bin/bash

# a quick script to render a talk agenda using OTel

svc="SRECon"

# turns out the date doesn't matter much for this exercise since
# we don't show it
# but it is important for /finding/ these spans once you've sent them
# so use today's date and hour but munge the rest
talk_start=$(date +%Y-%m-%dT%H:00:00.00000Z)
talk_end=$(date +%Y-%m-%dT%H:00:40.00000Z)

tpfile=$(mktemp)
otel-cli span \
	--name "OpenTelemetry::Trace.tracer(BareMetal)" \
	--service "$svc" \
	--start "$talk_start" \
	--end "$talk_end" \
	--tp-print \
	--tp-export \
	--tp-carrier $tpfile

# set the traceparent
. $tpfile

otel-cli span \
	--name "Agenda" \
	--service "$svc" \
	--start "$(echo "$talk_start" |sed "s/:00:00/:01:00/")" \
	--end   "$(echo "$talk_start" |sed "s/:00:00/:02:00/")"

subtpfile=$(mktemp)
otel-cli span \
	--name "The Bad Old Days" \
	--service "$svc" \
	--start "$(echo "$talk_start" |sed "s/:00:00/:02:01/")" \
	--end   "$(echo "$talk_start" |sed "s/:00:00/:03:01/")"

subtpfile=$(mktemp)
otel-cli span \
	--name "Tracing All the Things!" \
	--service "$svc" \
	--start "$(echo "$talk_start" |sed "s/:00:00/:03:01/")" \
	--end   "$(echo "$talk_start" |sed "s/:00:00/:10:01/")" \
	--tp-export \
	--tp-carrier $subtpfile
. $subtpfile # next spans are under ^^
	otel-cli span \
		--name "OTel in the Metal API" \
		--service "$svc" \
		--start "$(echo "$talk_start" |sed "s/:00:00/:03:01/")" \
		--end   "$(echo "$talk_start" |sed "s/:00:00/:04:00/")"
	otel-cli span \
		--name "Fits and Starts" \
		--service "$svc" \
		--start "$(echo "$talk_start" |sed "s/:00:00/:04:01/")" \
		--end   "$(echo "$talk_start" |sed "s/:00:00/:07:00/")"
	otel-cli span \
		--name "Introducing the Metal SRE team" \
		--service "$svc" \
		--start "$(echo "$talk_start" |sed "s/:00:00/:07:01/")" \
		--end   "$(echo "$talk_start" |sed "s/:00:00/:10:00/")"

. $tpfile # back to top trace

subtpfile=$(mktemp)
otel-cli span \
	--name "Observability Onboarding" \
	--service "$svc" \
	--start "$(echo "$talk_start" |sed "s/:00:00/:10:01/")" \
	--end   "$(echo "$talk_start" |sed "s/:00:00/:15:01/")" \
	--tp-export \
	--tp-carrier $subtpfile
. $subtpfile # next spans are under ^^
	otel-cli span \
		--name "Order of Operations" \
		--service "$svc" \
		--start "$(echo "$talk_start" |sed "s/:00:00/:10:01/")" \
		--end   "$(echo "$talk_start" |sed "s/:00:00/:12:00/")"
	otel-cli span \
		--name "Mental Models" \
		--service "$svc" \
		--start "$(echo "$talk_start" |sed "s/:00:00/:12:01/")" \
		--end   "$(echo "$talk_start" |sed "s/:00:00/:14:00/")"
	otel-cli span \
		--name "How We Know We're on the Right Track" \
		--service "$svc" \
		--start "$(echo "$talk_start" |sed "s/:00:00/:14:01/")" \
		--end   "$(echo "$talk_start" |sed "s/:00:00/:16:00/")"

. $tpfile # back to top trace

subtpfile=$(mktemp)
otel-cli span \
	--name "Tracing Wins" \
	--service "$svc" \
	--start "$(echo "$talk_start" |sed "s/:00:00/:16:01/")" \
	--end   "$(echo "$talk_start" |sed "s/:00:00/:22:01/")" \
	--tp-export \
	--tp-carrier $subtpfile
. $subtpfile # next spans are under ^^
	otel-cli span \
		--name "Performance Project" \
		--service "$svc" \
		--start "$(echo "$talk_start" |sed "s/:00:00/:16:02/")" \
		--end   "$(echo "$talk_start" |sed "s/:00:00/:19:00/")"
	otel-cli span \
		--name "Sociotechnical Wins" \
		--service "$svc" \
		--start "$(echo "$talk_start" |sed "s/:00:00/:19:01/")" \
		--end   "$(echo "$talk_start" |sed "s/:00:00/:22:00/")"

. $tpfile # back to top trace

otel-cli span \
	--name "Tracing Bear Metal" \
	--service "$svc" \
	--start "$(echo "$talk_start" |sed "s/:00:00/:22:01/")" \
	--end   "$(echo "$talk_start" |sed "s/:00:00/:28:00/")"

otel-cli span \
	--name "Recap" \
	--service "$svc" \
	--start "$(echo "$talk_start" |sed "s/:00:00/:28:01/")" \
	--end   "$(echo "$talk_start" |sed "s/:00:00/:29:59/")"

otel-cli span \
	--name "Q & A" \
	--service "$svc" \
	--start "$(echo "$talk_start" |sed "s/:00:00/:30:00/")" \
	--end   "$(echo "$talk_start" |sed "s/:00:00/:40:00/")"
