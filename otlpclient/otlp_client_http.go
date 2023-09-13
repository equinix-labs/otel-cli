package otlpclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"
)

// HttpClient holds state information for HTTP/OTLP.
type HttpClient struct {
	client *http.Client
	config OTLPConfig
}

// NewHttpClient returns an initialized HttpClient.
func NewHttpClient(config OTLPConfig) *HttpClient {
	c := HttpClient{config: config}
	return &c
}

// Start sets up the client configuration.
// TODO: see if there's a way to background start http2 connections?
func (hc *HttpClient) Start(ctx context.Context) (context.Context, error) {
	if hc.config.GetInsecure() {
		hc.client = &http.Client{Timeout: hc.config.GetTimeout()}
	} else {
		hc.client = &http.Client{
			Timeout: hc.config.GetTimeout(),
			Transport: &http.Transport{
				DialTLS: func(network, addr string) (net.Conn, error) {
					return tls.Dial(network, addr, hc.config.GetTlsConfig())
				},
			},
		}
	}
	return ctx, nil
}

// UploadTraces sends the protobuf spans up to the HTTP server.
func (hc *HttpClient) UploadTraces(ctx context.Context, rsps []*tracepb.ResourceSpans) (context.Context, error) {
	msg := coltracepb.ExportTraceServiceRequest{ResourceSpans: rsps}
	protoMsg, err := proto.Marshal(&msg)
	if err != nil {
		return ctx, fmt.Errorf("failed to marshal trace service request: %w", err)
	}
	body := bytes.NewBuffer(protoMsg)

	endpointURL := hc.config.GetEndpoint()
	req, err := http.NewRequest("POST", endpointURL.String(), body)
	if err != nil {
		return ctx, fmt.Errorf("failed to create HTTP POST request: %w", err)
	}

	for k, v := range hc.config.GetHeaders() {
		req.Header.Add(k, v)
	}
	req.Header.Set("Content-Type", "application/x-protobuf")

	return retry(ctx, hc.config, func(context.Context) (context.Context, bool, time.Duration, error) {
		var body []byte
		resp, err := hc.client.Do(req)
		if uerr, ok := err.(*url.Error); ok {
			// e.g. http on https, un-retriable error, quit now
			return ctx, false, 0, uerr
		} else {
			body, err = io.ReadAll(resp.Body)
			if err != nil {
				return ctx, true, 0, fmt.Errorf("io.Readall of response body failed: %w", err)
			}
			resp.Body.Close()

			return processHTTPStatus(ctx, resp, body)
		}
	})
}

// processHTTPStatus takes the http.Response and body, returning the same bool, error
// as retryFunc. Mostly it's broken out so it can be unit tested.
func processHTTPStatus(ctx context.Context, resp *http.Response, body []byte) (context.Context, bool, time.Duration, error) {
	// #262 a vendor OTLP server is out of spec and returns JSON instead of protobuf
	if ct, ok := resp.Header["Content-Type"]; ok {
		if len(ct) != 1 || ct[0] != "application/x-protobuf" {
			return ctx, false, 0, fmt.Errorf("server is out of specification: expected content type application/x-protobuf but got %q", ct[0])
		}
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// success & partial success
		// spec says server MUST send 200 OK, we'll be generous and accept any 200
		etsr := coltracepb.ExportTraceServiceResponse{}
		err := proto.Unmarshal(body, &etsr)
		if err != nil {
			// if the server's sending garbage, no point in retrying
			return ctx, false, 0, fmt.Errorf("unmarshal of server response failed: %w", err)
		}

		if partial := etsr.GetPartialSuccess(); partial != nil {
			// spec says to stop retrying and drop rejected spans
			return ctx, false, 0, fmt.Errorf("partial success. %d spans were rejected", partial.GetRejectedSpans())

		} else {
			// full success!
			return ctx, false, 0, nil
		}
	} else if resp.StatusCode == 429 || resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504 {
		// 429, 502, 503, and 504 must be retried according to spec
		return ctx, true, 0, fmt.Errorf("server responded with retriable code %d", resp.StatusCode)
	} else if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		// spec doesn't say anything about 300's, ignore body and assume they're errors and unretriable
		return ctx, false, 0, fmt.Errorf("server returned unsupported code %d", resp.StatusCode)
	} else if resp.StatusCode >= 400 {
		// https://github.com/open-telemetry/opentelemetry-proto/blob/main/docs/specification.md#failures-1
		st := status.Status{}
		err := proto.Unmarshal(body, &st)
		if err != nil {
			return ctx, false, 0, fmt.Errorf("unmarshal of server status failed: %w", err)
		} else {
			return ctx, false, 0, fmt.Errorf("server returned unretriable code %d with status: %s", resp.StatusCode, st.GetMessage())
		}
	}

	// should never happen
	return ctx, false, 0, fmt.Errorf("BUG: fell through error checking with status code %d", resp.StatusCode)
}

// Stop does nothing for HTTP, for now. It exists to fulfill the interface.
func (hc *HttpClient) Stop(ctx context.Context) (context.Context, error) {
	return ctx, nil
}
