package otelcli

import (
	"bytes"
	"context"
	"crypto/tls"
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
	}
	return nil
}

// UploadTraces sends the protobuf spans up to the HTTP server.
func (hc *HttpClient) UploadTraces(ctx context.Context, rsps []*tracepb.ResourceSpans) error {
	msg := coltracepb.ExportTraceServiceRequest{ResourceSpans: rsps}
	protoMsg, err := proto.Marshal(&msg)
	if err != nil {
		return err // TODO: beef up errors
	}
	body := bytes.NewBuffer(protoMsg)

	endpointURL, _ := parseEndpoint(hc.config)
	req, err := http.NewRequest("POST", endpointURL.String(), body)
	if err != nil {
		return err // TODO: beef up errors
	}

	for k, v := range config.Headers {
		req.Header.Add(k, v)
	}
	req.Header.Set("Content-Type", "application/x-protobuf")

	// TODO: look at the response
	return retry(hc.timeout, func() (bool, error) {
		_, err := hc.client.Do(req)
		// e.g. http on https, un-retriable error, quit now
		if uerr, ok := err.(*url.Error); ok {
			return false, uerr
		}
		return true, err
	})
}

// Stop does nothing for HTTP, for now. It exists to fulfill the interface.
func (hc *HttpClient) Stop(ctx context.Context) error {
	return nil
}
