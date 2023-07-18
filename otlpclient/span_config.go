package otlpclient

import (
	"time"

	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// NewProtobufSpan creates a new span and populates it with information
// from the provided config struct.
func (c Config) NewProtobufSpan() *tracepb.Span {
	span := NewProtobufSpan()
	if c.GetIsRecording() {
		span.TraceId = generateTraceId()
		span.SpanId = generateSpanId()
	}
	span.Name = c.SpanName
	span.Kind = SpanKindStringToInt(c.Kind)
	span.Attributes = StringMapAttrsToProtobuf(c.Attributes)

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
		tp := LoadTraceparent(c, span)
		if tp.Initialized {
			span.TraceId = tp.TraceId
			span.ParentSpanId = tp.SpanId
		}
	} else {
		span.TraceId = emptyTraceId
		span.SpanId = emptySpanId
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

	SetSpanStatus(span, c.StatusCode, c.StatusDescription)

	return span
}
