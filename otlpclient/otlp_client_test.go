package otlpclient

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestErrorLists(t *testing.T) {
	now := time.Now()

	for _, tc := range []struct {
		call func(context.Context) context.Context
		want ErrorList
	}{
		{
			call: func(ctx context.Context) context.Context {
				err := fmt.Errorf("")
				ctx, _ = SaveError(ctx, err)
				return ctx
			},
			want: ErrorList{
				TimestampedError{now, ""},
			},
		},
	} {
		ctx := context.Background()
		ctx = tc.call(ctx)
		list := GetErrorList(ctx)

		if len(list) < len(tc.want) {
			t.Error("not enough entries")
		}
	}
}

func TestParseEndpoint(t *testing.T) {
	// func parseEndpoint(config Config) (*url.URL, string) {

	for _, tc := range []struct {
		config       Config
		wantEndpoint string
		wantSource   string
	}{
		// gRPC, general, bare host
		{
			config:       DefaultConfig().WithEndpoint("localhost"),
			wantEndpoint: "grpc://localhost:4317",
			wantSource:   "general",
		},
		// gRPC, general, should be bare host:port
		{
			config:       DefaultConfig().WithEndpoint("localhost:4317"),
			wantEndpoint: "grpc://localhost:4317",
			wantSource:   "general",
		},
		// gRPC, general, https URL, should transform to host:port
		{
			config:       DefaultConfig().WithEndpoint("https://localhost:4317").WithProtocol("grpc"),
			wantEndpoint: "https://localhost:4317/v1/traces",
			wantSource:   "general",
		},
		// HTTP, general, with a provided default signal path, should not be modified
		{
			config:       DefaultConfig().WithEndpoint("http://localhost:9999/v1/traces"),
			wantEndpoint: "http://localhost:9999/v1/traces",
			wantSource:   "general",
		},
		// HTTP, general, with a provided custom signal path, signal path should get appended
		{
			config:       DefaultConfig().WithEndpoint("http://localhost:9999/my/collector/path"),
			wantEndpoint: "http://localhost:9999/my/collector/path/v1/traces",
			wantSource:   "general",
		},
		// HTTPS, general, without path, should get /v1/traces appended
		{
			config:       DefaultConfig().WithEndpoint("https://localhost:4317"),
			wantEndpoint: "https://localhost:4317/v1/traces",
			wantSource:   "general",
		},
		// gRPC, signal, should come through with just the grpc:// added
		{
			config:       DefaultConfig().WithTracesEndpoint("localhost"),
			wantEndpoint: "grpc://localhost:4317",
			wantSource:   "signal",
		},
		// http, signal, should come through unmodified
		{
			config:       DefaultConfig().WithTracesEndpoint("http://localhost"),
			wantEndpoint: "http://localhost",
			wantSource:   "signal",
		},
	} {
		u, src := ParseEndpoint(tc.config)

		if u.String() != tc.wantEndpoint {
			t.Errorf("Expected endpoint %q but got %q", tc.wantEndpoint, u.String())
		}

		if src != tc.wantSource {
			t.Errorf("Expected source %q for test url %q but got %q", tc.wantSource, u.String(), src)
		}
	}
}
