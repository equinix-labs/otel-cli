package otelcli

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// HttpClient holds state information for HTTP/OTLP.
type HttpClient struct {
	client  *http.Client
	config  Config
	timeout time.Duration
}

// NewHttpClient returns an initialized HttpClient.
func NewHttpClient(config Config) *HttpClient {
	c := HttpClient{config: config}
	return &c
}

// Start sets up the client configuration.
// TODO: see if there's a way to background start http2 connections?
func (hc *HttpClient) Start(ctx context.Context) error {
	tlsConf := tlsConfig(hc.config)
	hc.timeout = parseCliTimeout(hc.config)

	endpointURL, _ := parseEndpoint(config)
	if config.Insecure || (isLoopbackAddr(endpointURL) && !strings.HasPrefix(config.Endpoint, "https")) {
		hc.client = &http.Client{Timeout: hc.timeout}
	} else if !isInsecureSchema(config.Endpoint) {
		hc.client = &http.Client{
			Timeout: hc.timeout,
			Transport: &http.Transport{
				DialTLS: func(network, addr string) (net.Conn, error) {
					return tls.Dial(network, addr, tlsConf)
				},
			},
		}
	} else {
		softFail("BUG in otel-cli: an invalid configuration made it too far. Please report to https://github.com/equinix-labs/otel-cli/issues.")
	}
	return nil
}

// UploadTraces sends the protobuf spans up to the HTTP server.
func (hc *HttpClient) UploadTraces(ctx context.Context, rsps []*tracepb.ResourceSpans) error {
	msg := coltracepb.ExportTraceServiceRequest{ResourceSpans: rsps}
	protoMsg, err := proto.Marshal(&msg)
	if err != nil {
		return fmt.Errorf("failed to marshal trace service request: %w", err)
	}
	body := bytes.NewBuffer(protoMsg)

	endpointURL, _ := parseEndpoint(hc.config)
	req, err := http.NewRequest("POST", endpointURL.String(), body)
	if err != nil {
		return fmt.Errorf("failed to create HTTP POST request: %w", err)
	}

	for k, v := range config.Headers {
		req.Header.Add(k, v)
	}
	req.Header.Set("Content-Type", "application/x-protobuf")

	// TODO: look at the response
	return retry(hc.timeout, func() (bool, error) {
		resp, err := hc.client.Do(req)
		if uerr, ok := err.(*url.Error); ok {
			// e.g. http on https, un-retriable error, quit now
			return false, uerr
		} else if err != nil {
			// all other errors get retried
			return true, err
		} else {
			// success!
			io.ReadAll(resp.Body) // TODO, do something with body
			resp.Body.Close()
			return false, nil
		}
	})
}

// Stop does nothing for HTTP, for now. It exists to fulfill the interface.
func (hc *HttpClient) Stop(ctx context.Context) error {
	return nil
}
