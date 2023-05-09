package otlpserver

import (
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"

	colv1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	v1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

// CliEvent is a span or event decoded & copied for human consumption. It's roughly
// an OpenTelemetry trace unwrapped just enough to be usable in tools like otel-cli.
type CliEvent struct {
	TraceID           string            `json:"trace_id"`
	SpanID            string            `json:"span_id"`
	Parent            string            `json:"parent_span_id"`
	Scope             string            `json:"scope"`
	Name              string            `json:"name"`
	Kind              string            `json:"kind"`
	Start             time.Time         `json:"start"`
	End               time.Time         `json:"end"`
	ElapsedMs         int64             `json:"elapsed_ms"`
	Attributes        map[string]string `json:"attributes"`
	ServiceAttributes map[string]string `json:"service_attributes"`
	StatusCode        int32
	StatusDescription string
	// for a span this is the start nanos, for an event it's just the timestamp
	// mostly here for sorting CliEventList but could be any uint64
	Nanos uint64 `json:"nanos"`
	// the methods below will set this to true before returning
	// to make it easy for consumers to tell if they got a zero value
	IsPopulated bool `json:"has_been_modified"`
	// somewhere for the server to put interesting facts about a span
	// like what HTTP path it arrived on, what http method, etc.
	ServerMeta map[string]string `json:"server_meta"`
}

// ToStringMap flattens a CliEvent into a string map for testing.
func (ce CliEvent) ToStringMap() map[string]string {
	// time.UnixNano() is undefined for zero value so we have to check
	var stime, etime string
	if ce.Start.IsZero() {
		stime = "0"
	} else {
		stime = strconv.FormatInt(ce.Start.UnixNano(), 10)
	}
	if ce.End.IsZero() {
		etime = "0"
	} else {
		etime = strconv.FormatInt(ce.End.UnixNano(), 10)
	}

	return map[string]string{
		"trace_id":           ce.TraceID,
		"span_id":            ce.SpanID,
		"parent":             ce.Parent,
		"scope":              ce.Scope,
		"name":               ce.Name,
		"kind":               ce.Kind,
		"start":              stime,
		"end":                etime,
		"attributes":         mapToKVString(ce.Attributes),
		"service_attributes": mapToKVString(ce.ServiceAttributes),
		"status_code":        strconv.FormatInt(int64(ce.StatusCode), 10),
		"status_description": ce.StatusDescription,
		"is_populated":       strconv.FormatBool(ce.IsPopulated),
		"server_meta":        mapToKVString(ce.ServerMeta),
	}
}

// CliEventList implements sort.Interface for []CliEvent sorted by Nanos.
type CliEventList []CliEvent

func (cel CliEventList) Len() int           { return len(cel) }
func (cel CliEventList) Swap(i, j int)      { cel[i], cel[j] = cel[j], cel[i] }
func (cel CliEventList) Less(i, j int) bool { return cel[i].Nanos < cel[j].Nanos }

// NewCliEventFromSpan converts a raw grpc span into a CliEvent.
func NewCliEventFromSpan(span *v1.Span, scopeSpans *v1.ScopeSpans, rss *v1.ResourceSpans, serverMeta map[string]string) CliEvent {
	e := CliEvent{
		TraceID:           hex.EncodeToString(span.GetTraceId()),
		SpanID:            hex.EncodeToString(span.GetSpanId()),
		Parent:            hex.EncodeToString(span.GetParentSpanId()),
		Scope:             scopeSpans.GetScope().Name,
		Start:             time.Unix(0, int64(span.GetStartTimeUnixNano())),
		End:               time.Unix(0, int64(span.GetEndTimeUnixNano())),
		ElapsedMs:         int64((span.GetEndTimeUnixNano() - span.GetStartTimeUnixNano()) / 1000000),
		Name:              span.GetName(),
		Attributes:        make(map[string]string),
		ServiceAttributes: make(map[string]string),
		Nanos:             span.GetStartTimeUnixNano(),
		StatusCode:        int32(span.GetStatus().Code),
		StatusDescription: span.GetStatus().Message,
		IsPopulated:       true,
		ServerMeta:        make(map[string]string),
	}

	// copy service attributes over by string, which includes service.name
	for _, rattr := range rss.GetResource().GetAttributes() {
		e.ServiceAttributes[rattr.GetKey()] = rattr.Value.GetStringValue()
	}

	switch span.GetKind() {
	case v1.Span_SPAN_KIND_CLIENT:
		e.Kind = "client"
	case v1.Span_SPAN_KIND_SERVER:
		e.Kind = "server"
	case v1.Span_SPAN_KIND_PRODUCER:
		e.Kind = "producer"
	case v1.Span_SPAN_KIND_CONSUMER:
		e.Kind = "consumer"
	case v1.Span_SPAN_KIND_INTERNAL:
		e.Kind = "internal"
	default:
		e.Kind = "unspecified"
	}

	for _, attr := range span.GetAttributes() {
		// TODO: this isn't great, there are ways it can cause mayhem, but
		// should fine for known otlpserver use cases
		val := attr.Value.String()
		parts := strings.SplitN(val, "_value:", 2)
		if parts[0] != "" && parts[1] != "" {
			val = strings.Trim(parts[1], "\"")
		}
		e.Attributes[attr.GetKey()] = val
	}

	// explicitly copy the map, do not share the ref
	for k, v := range serverMeta {
		e.ServerMeta[k] = v
	}

	return e
}

// NewCliEventFromSpanEvent takes a span event, span, and ils and returns an event
// with all the span event info filled in.
func NewCliEventFromSpanEvent(se *v1.Span_Event, span *v1.Span, scopeSpans *v1.ScopeSpans, serverMeta map[string]string) CliEvent {
	// start with the span, rewrite it for the event
	e := CliEvent{
		TraceID:     hex.EncodeToString(span.GetTraceId()),
		SpanID:      hex.EncodeToString(span.GetSpanId()),
		Parent:      hex.EncodeToString(span.GetSpanId()),
		Scope:       scopeSpans.GetScope().Name,
		Kind:        "event",
		Start:       time.Unix(0, int64(se.GetTimeUnixNano())),
		End:         time.Unix(0, int64(se.GetTimeUnixNano())),
		ElapsedMs:   int64(se.GetTimeUnixNano()-span.GetStartTimeUnixNano()) / 1000000,
		Name:        se.GetName(),
		Attributes:  make(map[string]string), // overwrite the one from the span
		Nanos:       se.GetTimeUnixNano(),
		IsPopulated: true,
		ServerMeta:  make(map[string]string),
	}

	for _, attr := range se.GetAttributes() {
		e.Attributes[attr.GetKey()] = attr.Value.String()
	}

	// explicitly copy the map, do not share the ref
	for k, v := range serverMeta {
		e.ServerMeta[k] = v
	}

	return e
}

// otelToCliEvent takes an otel trace request data structure and converts
// it to CliEvents, calling the provided callback for each span in the
// request.
func otelToCliEvent(cb Callback, req *colv1.ExportTraceServiceRequest, serverMeta map[string]string) bool {
	rss := req.GetResourceSpans()
	for _, resource := range rss {
		scopeSpans := resource.GetScopeSpans()
		for _, ss := range scopeSpans {
			for _, span := range ss.GetSpans() {
				// convert protobuf spans to something easier for humans to consume
				ces := NewCliEventFromSpan(span, ss, resource, serverMeta)
				events := CliEventList{}
				for _, se := range span.GetEvents() {
					events = append(events, NewCliEventFromSpanEvent(se, span, ss, serverMeta))
				}

				done := cb(ces, events)
				if done {
					return true
				}
			}
		}
	}

	return false
}

// mapToKVString flattens attribute string maps into "k=v,k=v" strings.
func mapToKVString(in map[string]string) string {
	keys := make([]string, len(in)) // for sorting
	var i int
	for k := range in {
		keys[i] = k
		i++
	}

	sort.Strings(keys) // make output relatively consistent

	outs := make([]string, len(in))
	for i, k := range keys {
		outs[i] = k + "=" + in[k]
	}

	return strings.Join(outs, ",")
}
