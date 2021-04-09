FROM golang:latest AS builder

RUN mkdir /build
WORKDIR /build
COPY . .
ENV CGO_ENABLED=0
RUN go build -o otel-cli .

FROM scratch AS otel-cli

COPY --from=builder /build/otel-cli /otel-cli

ENTRYPOINT ["/otel-cli"]

