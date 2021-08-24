package cmd

import (
	"context"
	"encoding/hex"
	"log"
	"net"
	"time"

	"github.com/spf13/cobra"
	v1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"

	"google.golang.org/grpc"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "run an embedded OTLP server",
	Long:  "Run otel-cli as an OTLP server. See subcommands.",
}

func init() {
	rootCmd.AddCommand(serverCmd)
}

// serverCallback is a type for the function passed to newServer that is
// called for each incoming span
type serverCallback func(CliEvent, CliEventList) bool

// serverStop is the function passed to newServer to be called when the
// server is shut down.
type serverStop func(*cliServer)

// cliServer is a gRPC/OTLP server handle.
type cliServer struct {
	server   *grpc.Server
	callback serverCallback
	stopper  chan bool
	v1.UnimplementedTraceServiceServer
}

// newServer takes a callback and stop function and returns a cliServer ready
// to run with .ServeGRPC().
func newServer(cb serverCallback, stop serverStop) *cliServer {
	cs := cliServer{
		server:   grpc.NewServer(),
		callback: cb,
		stopper:  make(chan bool),
	}

	v1.RegisterTraceServiceServer(cs.server, &cs)

	// single place to stop the server, used by timeout and max-spans
	go func() {
		<-cs.stopper
		stop(&cs)
		cs.server.Stop()
	}()

	return &cs
}

// ServeGRPC creates a listener on otlpEndpoint and starts the GRPC server
// on that listener. Blocks until stopped by sending a value to cs.stopper.
func (cs *cliServer) ServeGPRC() {
	listener, err := net.Listen("tcp", otlpEndpoint)
	if err != nil {
		log.Fatalf("failed to listen: %s", err)
	}

	if err := cs.server.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}

// Stop sends a value to the server shutdown goroutine so it stops GRPC
// and calls the stop function given to newServer.
func (cs *cliServer) Stop() {
	cs.stopper <- true
}

// Export implements the gRPC server interface for exporting messages.
func (cs *cliServer) Export(ctx context.Context, req *v1.ExportTraceServiceRequest) (*v1.ExportTraceServiceResponse, error) {
	rss := req.GetResourceSpans()
	for _, resource := range rss {
		ilSpans := resource.GetInstrumentationLibrarySpans()
		for _, ils := range ilSpans {
			for _, span := range ils.GetSpans() {
				// convert protobuf spans to something easier for humans to consume
				ces := newCliEventFromSpan(span, ils)
				events := CliEventList{}
				for _, se := range span.GetEvents() {
					events = append(events, newCliEventFromSpanEvent(se, span, ils))
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
	nanos      uint64            // only used to sort
}

// CliEventList implements sort.Interface for []CliEvent sorted by time
type CliEventList []CliEvent

func (cel CliEventList) Len() int           { return len(cel) }
func (cel CliEventList) Swap(i, j int)      { cel[i], cel[j] = cel[j], cel[i] }
func (cel CliEventList) Less(i, j int) bool { return cel[i].nanos < cel[j].nanos }

// newCliEventFromSpan converts a raw span into a CliEvent.
func newCliEventFromSpan(span *tracepb.Span, ils *tracepb.InstrumentationLibrarySpans) CliEvent {
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
		nanos:      span.GetStartTimeUnixNano(),
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

// newCliEventFromSpanEvent takes a span event, span, and ils and returns an event
// with all the span event info filled in
func newCliEventFromSpanEvent(se *tracepb.Span_Event, span *tracepb.Span, ils *tracepb.InstrumentationLibrarySpans) CliEvent {
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
		nanos:      se.GetTimeUnixNano(),
	}

	for _, attr := range se.GetAttributes() {
		e.Attributes[attr.GetKey()] = attr.Value.String()
	}

	return e
}
