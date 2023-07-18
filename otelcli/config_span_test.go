package otelcli

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"testing"

	"github.com/equinix-labs/otel-cli/otlpclient"
)

func TestPropagateTraceparent(t *testing.T) {
	config := DefaultConfig().
		WithTraceparentCarrierFile("").
		WithTraceparentPrint(false).
		WithTraceparentPrintExport(false)

	tp := "00-3433d5ae39bdfee397f44be5146867b3-8a5518f1e5c54d0a-01"
	tid := "3433d5ae39bdfee397f44be5146867b3"
	sid := "8a5518f1e5c54d0a"
	os.Setenv("TRACEPARENT", tp)

	span := otlpclient.NewProtobufSpan()
	span.TraceId, _ = hex.DecodeString(tid)
	span.SpanId, _ = hex.DecodeString(sid)

	buf := new(bytes.Buffer)
	config.PropagateTraceparent(span, buf)
	if buf.Len() != 0 {
		t.Errorf("nothing was supposed to be written but %d bytes were", buf.Len())
	}

	config.TraceparentPrint = true
	config.TraceparentPrintExport = true
	buf = new(bytes.Buffer)
	config.PropagateTraceparent(span, buf)
	if buf.Len() == 0 {
		t.Error("expected more than zero bytes but got none")
	}
	expected := fmt.Sprintf("# trace id: %s\n#  span id: %s\nexport TRACEPARENT=%s\n", tid, sid, tp)
	if buf.String() != expected {
		t.Errorf("got unexpected output, expected '%s', got '%s'", expected, buf.String())
	}
}

func TestNewProtobufSpanWithConfig(t *testing.T) {
	c := DefaultConfig().WithSpanName("test span 123")
	span := c.NewProtobufSpan()

	if span.Name != "test span 123" {
		t.Error("span event attributes must not be nil")
	}
}
