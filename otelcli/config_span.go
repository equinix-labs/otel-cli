package otelcli

import (
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/tobert/otel-cli/otlpclient"
	"github.com/tobert/otel-cli/w3c/traceparent"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// NewProtobufSpan creates a new span and populates it with information
// from the config struct.
func (c Config) NewProtobufSpan() *tracepb.Span {
	span := otlpclient.NewProtobufSpan()
	if c.GetIsRecording() {
		span.TraceId = otlpclient.GenerateTraceId()
		span.SpanId = otlpclient.GenerateSpanId()
	}
	span.Name = c.SpanName
	span.Kind = otlpclient.SpanKindStringToInt(c.Kind)
	span.Attributes = otlpclient.StringMapAttrsToProtobuf(c.Attributes)

	now := time.Now()
	if c.SpanStartTime != "" {
		st := c.ParseSpanStartTime()
		span.StartTimeUnixNano = uint64(st.UnixNano())
	} else {
		span.StartTimeUnixNano = uint64(now.UnixNano())
	}

	if c.SpanEndTime != "" {
		et := c.ParseSpanEndTime()
		span.EndTimeUnixNano = uint64(et.UnixNano())
	} else {
		span.EndTimeUnixNano = uint64(now.UnixNano())
	}

	if c.GetIsRecording() {
		tp := c.LoadTraceparent()
		if tp.Initialized {
			span.TraceId = tp.TraceId
			span.ParentSpanId = tp.SpanId
		}
	} else {
		span.TraceId = otlpclient.GetEmptyTraceId()
		span.SpanId = otlpclient.GetEmptySpanId()
	}

	// --force-trace-id, --force-span-id and --force-parent-span-id let the user set their own trace, span & parent span ids
	// these work in non-recording mode and will stomp trace id from the traceparent
	var err error
	if c.ForceTraceId != "" {
		span.TraceId, err = parseHex(c.ForceTraceId, 16)
		c.SoftFailIfErr(err)
	}
	if c.ForceSpanId != "" {
		span.SpanId, err = parseHex(c.ForceSpanId, 8)
		c.SoftFailIfErr(err)
	}
	if c.ForceParentSpanId != "" {
		span.ParentSpanId, err = parseHex(c.ForceParentSpanId, 8)
		c.SoftFailIfErr(err)
	}

	otlpclient.SetSpanStatus(span, c.StatusCode, c.StatusDescription)

	return span
}

// LoadTraceparent follows otel-cli's loading rules, start with envvar then file.
// If both are set, the file will override env.
// When in non-recording mode, the previous traceparent will be returned if it's
// available, otherwise, a zero-valued traceparent is returned.
func (c Config) LoadTraceparent() traceparent.Traceparent {
	tp := traceparent.Traceparent{
		Version:     0,
		TraceId:     otlpclient.GetEmptyTraceId(),
		SpanId:      otlpclient.GetEmptySpanId(),
		Sampling:    false,
		Initialized: true,
	}

	if !c.TraceparentIgnoreEnv {
		var err error
		tp, err = traceparent.LoadFromEnv()
		if err != nil {
			Diag.Error = err.Error()
		}
	}

	if c.TraceparentCarrierFile != "" {
		fileTp, err := traceparent.LoadFromFile(c.TraceparentCarrierFile)
		if err != nil {
			Diag.Error = err.Error()
		} else if fileTp.Initialized {
			tp = fileTp
		}
	}

	if c.TraceparentRequired {
		if tp.Initialized {
			return tp
		} else {
			c.SoftFail("failed to find a valid traceparent carrier in either environment for file '%s' while it's required by --tp-required", c.TraceparentCarrierFile)
		}
	}

	return tp
}

// PropagateTraceparent saves the traceparent to file if necessary, then prints
// span info to the console according to command-line args.
func (c Config) PropagateTraceparent(span *tracepb.Span, target io.Writer) {
	var tp traceparent.Traceparent
	if c.GetIsRecording() {
		tp = otlpclient.TraceparentFromProtobufSpan(span, c.GetIsRecording())
	} else {
		// when in non-recording mode, and there is a TP available, propagate that
		tp = c.LoadTraceparent()
	}

	if c.TraceparentCarrierFile != "" {
		err := tp.SaveToFile(c.TraceparentCarrierFile, c.TraceparentPrintExport)
		c.SoftFailIfErr(err)
	}

	if c.TraceparentPrint {
		tp.Fprint(target, c.TraceparentPrintExport)
	}
}

// parseHex parses hex into a []byte of length provided. Errors if the input is
// not valid hex or the converted hex is not the right number of bytes.
func parseHex(in string, expectedLen int) ([]byte, error) {
	out, err := hex.DecodeString(in)
	if err != nil {
		return nil, fmt.Errorf("error parsing hex string %q: %w", in, err)
	}
	if len(out) != expectedLen {
		return nil, fmt.Errorf("hex string %q is the wrong length, expected %d bytes but got %d", in, expectedLen, len(out))
	}
	return out, nil
}
