package otlpserver

import (
	"encoding/hex"
	"time"

	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// CliEvent is a span or event decoded & copied for human consumption. It's roughly
// an OpenTelemetry trace unwrapped just enough to be usable in tools like otel-cli.
type CliEvent struct {
	TraceID    string            `json:"trace_id"`
	SpanID     string            `json:"span_id"`
	Parent     string            `json:"parent_span_id"`
	Library    string            `json:"library"`
	Name       string            `json:"name"`
	Kind       string            `json:"kind"`
	Start      time.Time         `json:"start"`
	End        time.Time         `json:"end"`
	ElapsedMs  int64             `json:"elapsed_ms"`
	Attributes map[string]string `json:"attributes"`
	// for a span this is the start nanos, for an event it's just the timestamp
	// mostly here for sorting CliEventList but could be any uint64
	Nanos uint64 `json:"nanos"`
}

// CliEventList implements sort.Interface for []CliEvent sorted by Nanos.
type CliEventList []CliEvent

func (cel CliEventList) Len() int           { return len(cel) }
func (cel CliEventList) Swap(i, j int)      { cel[i], cel[j] = cel[j], cel[i] }
func (cel CliEventList) Less(i, j int) bool { return cel[i].Nanos < cel[j].Nanos }

// NewCliEventFromSpan converts a raw grpc span into a CliEvent.
func NewCliEventFromSpan(span *tracepb.Span, ils *tracepb.InstrumentationLibrarySpans) CliEvent {
	e := CliEvent{
		TraceID:    hex.EncodeToString(span.GetTraceId()),
		SpanID:     hex.EncodeToString(span.GetSpanId()),
		Parent:     hex.EncodeToString(span.GetParentSpanId()),
		Library:    ils.InstrumentationLibrary.Name,
		Start:      time.Unix(0, int64(span.GetStartTimeUnixNano())),
		End:        time.Unix(0, int64(span.GetEndTimeUnixNano())),
		ElapsedMs:  int64((span.GetEndTimeUnixNano() - span.GetStartTimeUnixNano()) / 1000000),
		Name:       span.GetName(),
		Attributes: make(map[string]string),
		Nanos:      span.GetStartTimeUnixNano(),
	}

	switch span.GetKind() {
	case tracepb.Span_SPAN_KIND_CLIENT:
		e.Kind = "client"
	case tracepb.Span_SPAN_KIND_SERVER:
		e.Kind = "server"
	case tracepb.Span_SPAN_KIND_PRODUCER:
		e.Kind = "producer"
	case tracepb.Span_SPAN_KIND_CONSUMER:
		e.Kind = "consumer"
	case tracepb.Span_SPAN_KIND_INTERNAL:
		e.Kind = "internal"
	default:
		e.Kind = "unspecified"
	}

	for _, attr := range span.GetAttributes() {
		// TODO: break down by type so it doesn't return e.g. "int_value:99"
		e.Attributes[attr.GetKey()] = attr.Value.String()
	}

	return e
}

// NewCliEventFromSpanEvent takes a span event, span, and ils and returns an event
// with all the span event info filled in.
func NewCliEventFromSpanEvent(se *tracepb.Span_Event, span *tracepb.Span, ils *tracepb.InstrumentationLibrarySpans) CliEvent {
	// start with the span, rewrite it for the event
	e := CliEvent{
		TraceID:    hex.EncodeToString(span.GetTraceId()),
		SpanID:     hex.EncodeToString(span.GetSpanId()),
		Parent:     hex.EncodeToString(span.GetSpanId()),
		Library:    ils.InstrumentationLibrary.Name,
		Kind:       "event",
		Start:      time.Unix(0, int64(se.GetTimeUnixNano())),
		End:        time.Unix(0, int64(se.GetTimeUnixNano())),
		ElapsedMs:  int64(se.GetTimeUnixNano()-span.GetStartTimeUnixNano()) / 1000000,
		Name:       se.GetName(),
		Attributes: make(map[string]string), // overwrite the one from the span
		Nanos:      se.GetTimeUnixNano(),
	}

	for _, attr := range se.GetAttributes() {
		e.Attributes[attr.GetKey()] = attr.Value.String()
	}

	return e
}
