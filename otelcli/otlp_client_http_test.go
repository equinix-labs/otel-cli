package otelcli

import (
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// func processHTTPStatus(resp *http.Response, body []byte) (bool, error)
func TestProcessHTTPStatus(t *testing.T) {

	for _, tc := range []struct {
		resp      *http.Response
		body      []byte
		keepgoing bool
		err       error
	}{
		{
			resp:      &http.Response{},
			body:      []byte(""),
			keepgoing: false,
			err:       nil,
		},
	} {
		kg, err := processHTTPStatus(tc.resp, tc.body)

		if kg != tc.keepgoing {
			t.Errorf("keepgoing value returned %t but expected %t", kg, tc.keepgoing)
		}

		if tc.err == nil && err != nil {
			t.Errorf("error did not match testcase")
		} else if tc.err != nil && err == nil {
			t.Errorf("error did not match testcase")
		} else if tc.err == nil && err == nil {
			continue // pass
		} else if diff := cmp.Diff(tc.err.Error(), err.Error()); diff != "" {
			t.Errorf("error did not match testcase: %s", diff)
		}
	}
}
