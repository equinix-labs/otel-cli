# While the top-level Dockerfile is set up for local development on otel-cli,
# this Dockerfile is only for release. otel-cli containers should contain only
# the otel-cli static binary and nothing else.
FROM scratch
ENTRYPOINT ["/otel-cli"]
COPY otel-cli /
