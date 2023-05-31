package otelcli

import (
	"bytes"
	"context"
	"crypto/tls"
	"net"
	"net/http"

	v1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

type HttpClient struct {
	client *http.Client
	config Config
}

// TODO: pass config into this, for now it's matching the OTel interface
func NewHttpClient() *HttpClient {
	// passes in the global, this will go away after lifting off the OTel backend
	return RealNewHttpClient(config)
}

func RealNewHttpClient(config Config) *HttpClient {
	c := HttpClient{config: config}
	return &c
}

func (hc *HttpClient) Start(ctx context.Context) error {
	tlsConf := tlsConfig(hc.config)
	timeout := parseCliTimeout(hc.config)

	hc.client = &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialTLS: func(network, addr string) (net.Conn, error) {
				return tls.Dial(network, addr, tlsConf)
			},
		},
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

	resp, err := hc.client.Do(req)
	if err != nil {
		return err // TODO: beef up errors
	}

	softLog(resp.Status)

	return nil
}

func (hc *HttpClient) Stop(ctx context.Context) error {
	//return hc.conn.Close()
	return nil
}
