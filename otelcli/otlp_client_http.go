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

	v1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

type HttpClient struct {
	client  *http.Client
	config  Config
	timeout time.Duration
}

func NewHttpClient(config Config) *HttpClient {
	c := HttpClient{config: config}
	return &c
}

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

func (hc *HttpClient) UploadTraces(ctx context.Context, rsps []*tracepb.ResourceSpans) error {

	msg := v1.ExportTraceServiceRequest{ResourceSpans: rsps}
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
		if uerr, ok := err.(*url.Error); ok {
			return false, uerr
		}
		return true, err
	})
}

func (hc *HttpClient) Stop(ctx context.Context) error {
	//return hc.conn.Close()
	return nil
}
