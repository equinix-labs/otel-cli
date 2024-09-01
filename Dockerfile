FROM golang:alpine AS builder

WORKDIR /build
COPY . .
ENV CGO_ENABLED=0
RUN go build -ldflags="-w -s" -o otel-cli .

FROM scratch AS otel-cli

COPY --from=builder /build/otel-cli /otel-cli

ENTRYPOINT ["/otel-cli"]

