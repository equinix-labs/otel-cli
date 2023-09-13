package otlpclient

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestProcessHTTPStatus(t *testing.T) {
	headers := http.Header{
		"Content-Type": []string{"application/x-protobuf"},
	}

	for _, tc := range []struct {
		resp      *http.Response
		body      []byte
		keepgoing bool
		err       error
	}{
		// simple success
		{
			resp: &http.Response{
				StatusCode: 200,
				Header:     headers,
			},
			body:      etsrSuccessBody(),
			keepgoing: false,
			err:       nil,
		},
		// partial success
		{
			resp: &http.Response{
				StatusCode: 200,
				Header:     headers,
			},
			body:      etsrPartialSuccessBody(),
			keepgoing: false,
			err:       fmt.Errorf("partial success. 1 spans were rejected"),
		},
		// failure, unretriable
		{
			resp: &http.Response{
				StatusCode: 500,
				Header:     headers,
			},
			body:      errorBody(500, "xyz"),
			keepgoing: false,
			err:       fmt.Errorf("server returned unretriable code 500 with status: xyz"),
		},
		// failures the spec requires retries for, 429, 502, 503, 504
		{
			resp: &http.Response{
				StatusCode: 429,
				Header:     headers,
			},
			body:      errorBody(429, "xyz"),
			keepgoing: true,
			err:       fmt.Errorf("server responded with retriable code 429"),
		},
		{
			resp: &http.Response{
				StatusCode: 502,
				Header:     headers,
			},
			body:      errorBody(502, "xyz"),
			keepgoing: true,
			err:       fmt.Errorf("server responded with retriable code 502"),
		},
		{
			resp: &http.Response{
				StatusCode: 503,
				Header:     headers,
			},
			body:      errorBody(503, "xyz"),
			keepgoing: true,
			err:       fmt.Errorf("server responded with retriable code 503"),
		},
		{
			resp: &http.Response{
				StatusCode: 504,
				Header:     headers,
			},
			body:      errorBody(504, "xyz"),
			keepgoing: true,
			err:       fmt.Errorf("server responded with retriable code 504"),
		},
		// 300's are unsupported
		{
			resp: &http.Response{
				StatusCode: 301,
				Header:     headers,
			},
			body:      errorBody(301, "xyz"),
			keepgoing: false,
			err:       fmt.Errorf("server returned unsupported code 301"),
		},
		// shouldn't happen in the real world...
		{
			resp:      &http.Response{Header: headers},
			body:      []byte(""),
			keepgoing: false,
			err:       fmt.Errorf("BUG: fell through error checking with status code 0"),
		},
		// return a decent error for out-of-spec servers that return JSON after a protobuf payload
		{
			resp: &http.Response{
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			},
			body:      []byte(`{"some": "json"}`),
			keepgoing: false,
			err:       fmt.Errorf(`server is out of specification: expected content type application/x-protobuf but got "application/json"`),
		},
		// spec requires headers so report that as a server problem too
		{
			resp: &http.Response{
				StatusCode: 200,
				// no headers!
			},
			body:      []byte(""),
			keepgoing: false,
			err:       fmt.Errorf("server is out of specification: Content-Type header is missing or mangled"),
		},
	} {
		ctx := context.Background()
		_, kg, _, err := processHTTPStatus(ctx, tc.resp, tc.body)

		if kg != tc.keepgoing {
			t.Errorf("keepgoing value returned %t but expected %t", kg, tc.keepgoing)
		}

		if tc.err == nil && err != nil {
			t.Errorf("received an unexpected error")
		} else if tc.err != nil && err == nil {
			t.Errorf("did not receive expected error")
		} else if tc.err == nil && err == nil {
			continue // pass
		} else if diff := cmp.Diff(tc.err.Error(), err.Error()); diff != "" {
			t.Errorf("error did not match testcase: %s", diff)
		}
	}
}

func etsrSuccessBody() []byte {
	etsr := coltracepb.ExportTraceServiceResponse{
		PartialSuccess: nil,
	}
	b, _ := proto.Marshal(&etsr)
	return b
}

func etsrPartialSuccessBody() []byte {
	etsr := coltracepb.ExportTraceServiceResponse{
		PartialSuccess: &coltracepb.ExportTracePartialSuccess{
			RejectedSpans: 1,
			ErrorMessage:  "xyz",
		},
	}
	b, _ := proto.Marshal(&etsr)
	return b
}

func errorBody(c int32, message string) []byte {
	st := status.Status{
		Code:    c,
		Message: message,
		Details: []*anypb.Any{},
	}
	b, _ := proto.Marshal(&st)
	return b
}
