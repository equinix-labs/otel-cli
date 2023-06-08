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
	"google.golang.org/genproto/googleapis/rpc/status"
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

	return retry(hc.timeout, func() (bool, error) {
		var body []byte
		resp, err := hc.client.Do(req)
		if uerr, ok := err.(*url.Error); ok {
			// e.g. http on https, un-retriable error, quit now
			return false, uerr
		} else if err != nil {
			// all other errors get retried
			return true, err
		} else {
			body, err = io.ReadAll(resp.Body)
			if err != nil {
				return true, fmt.Errorf("io.Readall of response body failed: %w", err)
			}
			resp.Body.Close()

			return processHTTPStatus(resp, body)
		}
	})
}

// processHTTPStatus takes the http.Response and body, returning the same bool, error
// as retryFunc. Mostly it's broken out so it can be unit tested.
func processHTTPStatus(resp *http.Response, body []byte) (bool, error) {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// success & partial success
		// spec says server MUST send 200 OK, we'll be generous and accept any 200
		etsr := coltracepb.ExportTraceServiceResponse{}
		err := proto.Unmarshal(body, &etsr)
		if err != nil {
			// if the server's sending garbage, no point in retrying
			return false, fmt.Errorf("unmarshal of server response failed: %w", err)
		}

		if partial := etsr.GetPartialSuccess(); partial != nil {
			// spec says to stop retrying and drop rejected spans
			return false, fmt.Errorf("partial success. %d spans were rejected", partial.GetRejectedSpans())

		} else {
			// full success!
			return false, nil
		}
	} else if resp.StatusCode == 429 || resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504 {
		// 429, 502, 503, and 504 must be retried according to spec
		return true, fmt.Errorf("server responded with retriable code %d", resp.StatusCode)
	} else if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		// spec doesn't say anything 300's, ignore body and assume they're errors and unretriable
		return false, fmt.Errorf("server returned unsupported code %d", resp.StatusCode)
	} else if resp.StatusCode >= 400 {
		// https://github.com/open-telemetry/opentelemetry-proto/blob/main/docs/specification.md#failures-1
		st := status.Status{}
		err := proto.Unmarshal(body, &st)
		if err != nil {
			return false, fmt.Errorf("unmarshal of server status failed: %w", err)
		} else {
			return false, fmt.Errorf("server returned unretriable code %d with status: %s", resp.StatusCode, st.GetMessage())
		}
	}

	// should never happen
	return false, fmt.Errorf("BUG: fell through error checking with status code %d", resp.StatusCode)
}

// Stop does nothing for HTTP, for now. It exists to fulfill the interface.
func (hc *HttpClient) Stop(ctx context.Context) error {
	return nil
}
