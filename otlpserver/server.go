// otlpserver is a lightweight OTLP/gRPC server implementation intended for use
// in otel-cli and end-to-end testing of OpenTelemetry applications.
package otlpserver

import (
	"context"
	"encoding/hex"
	"log"
	"net"
	"time"

	v1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"

	"google.golang.org/grpc"
)

// Callback is a type for the function passed to newServer that is
// called for each incoming span
type Callback func(CliEvent, CliEventList) bool

// Stopper is the function passed to newServer to be called when the
// server is shut down.
type Stopper func(*Server)

// Server is a gRPC/OTLP server handle.
type Server struct {
	server   *grpc.Server
	callback Callback
	stopper  chan bool
	v1.UnimplementedTraceServiceServer
}

// newServer takes a callback and stop function and returns a Server ready
// to run with .ServeGRPC().
func NewServer(cb Callback, stop Stopper) *Server {
	s := Server{
		server:   grpc.NewServer(),
		callback: cb,
		stopper:  make(chan bool),
	}

	v1.RegisterTraceServiceServer(s.server, &s)

	// single place to stop the server, used by timeout and max-spans
	go func() {
		<-s.stopper
		stop(&s)
		s.server.Stop()
	}()

	return &s
}

// ServeGRPC creates a listener on otlpEndpoint and starts the GRPC server
// on that listener. Blocks until stopped by sending a value to cs.stopper.
func (cs *Server) ServeGPRC(endpoint string) {
	listener, err := net.Listen("tcp", endpoint)
	if err != nil {
		log.Fatalf("failed to listen on %q: %s", endpoint, err)
	}

	if err := cs.server.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}

// Stop sends a value to the server shutdown goroutine so it stops GRPC
// and calls the stop function given to newServer.
func (cs *Server) Stop() {
	cs.stopper <- true
}

// Export implements the gRPC server interface for exporting messages.
func (cs *Server) Export(ctx context.Context, req *v1.ExportTraceServiceRequest) (*v1.ExportTraceServiceResponse, error) {
	rss := req.GetResourceSpans()
	for _, resource := range rss {
		ilSpans := resource.GetInstrumentationLibrarySpans()
		for _, ils := range ilSpans {
			for _, span := range ils.GetSpans() {
				// convert protobuf spans to something easier for humans to consume
				ces := NewCliEventFromSpan(span, ils)
				events := CliEventList{}
				for _, se := range span.GetEvents() {
					events = append(events, NewCliEventFromSpanEvent(se, span, ils))
				}

				f := cs.callback
				done := f(ces, events)
				if done {
					return &v1.ExportTraceServiceResponse{}, nil
				}
			}
		}
	}

	return &v1.ExportTraceServiceResponse{}, nil
}

// Event is a span or event decoded & copied for human consumption.
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

// CliEventList implements sort.Interface for []CliEvent sorted by time
type CliEventList []CliEvent

func (cel CliEventList) Len() int           { return len(cel) }
func (cel CliEventList) Swap(i, j int)      { cel[i], cel[j] = cel[j], cel[i] }
func (cel CliEventList) Less(i, j int) bool { return cel[i].Nanos < cel[j].Nanos }

// NewCliEventFromSpan converts a raw span into a CliEvent.
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
// with all the span event info filled in
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
