# While the top-level Dockerfile is set up for local development on otel-cli,
# this Dockerfile is only for release.
#
# We use the Alpine base image to get the TLS trust store and not much else.
# The ca-certificates-bundle packet is pre-installed in the base so no
# additional packages are required.
FROM alpine:latest
ENTRYPOINT ["/otel-cli"]
COPY otel-cli /
