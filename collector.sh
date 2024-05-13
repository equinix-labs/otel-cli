#!/bin/bash

if [ -z "$HONEYCOMB_TEAM" ] ; then
	echo "you forgot to . ~/.hc"
	exit 1
fi

cat > "local.yaml" << EOF
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: "0.0.0.0:4317"
      http:
        endpoint: "0.0.0.0:4318"
processors:
  batch:
exporters:
  debug:
    verbosity: detailed
  otlp/0:
    endpoint: "api.honeycomb.io:443"
    headers:
      "x-honeycomb-team": "${HONEYCOMB_TEAM}"
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [debug,otlp/0]
EOF

docker rm -f otel-collector

docker run \
   --env HONEYCOMB_TEAM \
   --env HONEYCOMB_DATASET \
   --name otel-collector \
   --net host \
   --volume $(pwd)/local.yaml:/local.yaml \
   otel/opentelemetry-collector-contrib:0.101.0 \
      --config /local.yaml
