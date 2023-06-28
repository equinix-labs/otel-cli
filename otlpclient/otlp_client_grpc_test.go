package otlpclient

import (
	"context"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/google/go-cmp/cmp"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestProcessGrpcStatus(t *testing.T) {
	for i, tc := range []struct {
		etsr      *coltracepb.ExportTraceServiceResponse
		keepgoing bool
		err       error
		wait      time.Duration
	}{
		// simple success
		{
			etsr:      &coltracepb.ExportTraceServiceResponse{},
			keepgoing: false,
			err:       nil,
		},
		// partial success, no retry
		{
			etsr: &coltracepb.ExportTraceServiceResponse{
				PartialSuccess: &coltracepb.ExportTracePartialSuccess{
					RejectedSpans: 2,
					ErrorMessage:  "whoops",
				},
			},
			keepgoing: false,
			err:       status.Errorf(codes.OK, ""),
		},
		// failure, unretriable
		{
			etsr:      &coltracepb.ExportTraceServiceResponse{},
			keepgoing: false,
			err:       status.Errorf(codes.PermissionDenied, "test: permission denied"),
		},
		// failure, retry
		{
			etsr:      &coltracepb.ExportTraceServiceResponse{},
			keepgoing: true,
			err:       status.Errorf(codes.DeadlineExceeded, "test: should retry"),
		},
		// failure, retry, with server-provided wait
		{
			etsr:      &coltracepb.ExportTraceServiceResponse{},
			keepgoing: true,
			err:       retryWithInfo(1),
			wait:      time.Second,
		},
	} {
		ctx := context.Background()
		_, kg, wait, err := processGrpcStatus(ctx, tc.etsr, tc.err)

		if kg != tc.keepgoing {
			t.Errorf("keepgoing value returned %t but expected %t in test %d", kg, tc.keepgoing, i)
		}

		if tc.err == nil && err != nil {
			t.Errorf("received an unexpected error on test %d", i)
		} else if tc.err != nil && err == nil {
			t.Errorf("did not receive expected error on test %d", i)
		} else if tc.err == nil && err == nil {
			// success, do nothing
		} else if diff := cmp.Diff(tc.err.Error(), err.Error()); diff != "" {
			t.Errorf("error did not match testcase for test %d: %s", i, diff)
		}

		if wait != tc.wait {
			t.Errorf("expected a wait value of %d but got %d", tc.wait, wait)
		}
	}
}

func retryWithInfo(wait int64) error {
	var err error
	st := status.New(codes.ResourceExhausted, "Server unavailable")
	if wait > 0 {
		st, err = st.WithDetails(&errdetails.RetryInfo{
			RetryDelay: &duration.Duration{Seconds: wait},
		})
		if err != nil {
			panic("error creating retry info")
		}
	}

	return st.Err()
}
