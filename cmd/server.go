package cmd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	v1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"

	"google.golang.org/grpc"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "run a simple OTLP server",
	Long: `

outdir=$(mktemp -d)
otel-cli server -j $outdir

otel-cli server --json-out $outdir --max-spans 4 --timeout 30
otel-cli server --stdout
`,
	Run: doServer,
}

// serverConf holds the command-line configured settings for otel-cli server
var serverConf struct {
	outDir   string
	maxSpans int
	timeout  int
	verbose  bool
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().StringVar(&serverConf.outDir, "json-out", "", "write spans to json in the specified directory")
	serverCmd.Flags().IntVar(&serverConf.maxSpans, "max-spans", 0, "exit the server after this many spans come in")
	serverCmd.Flags().IntVar(&serverConf.timeout, "timeout", 0, "exit the server after timeout seconds")
	serverCmd.Flags().BoolVar(&serverConf.verbose, "verbose", false, "print a log every time a span comes in")
}

// cliServer is a gRPC/OTLP server handle.
type cliServer struct {
	spansSeen int
	stopper   chan bool
	v1.UnimplementedTraceServiceServer
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
				events := []CliEvent{}
				for _, se := range span.GetEvents() {
					events = append(events, newCliEventFromSpanEvent(se, span, ils))
				}

				if serverConf.outDir != "" {
					cs.writeFile(ces, events)
				}

				cs.spansSeen++ // count spans for exiting on --max-spans
				if serverConf.maxSpans > 0 && cs.spansSeen >= serverConf.maxSpans {
					cs.stopper <- true // shus the server down
					return &v1.ExportTraceServiceResponse{}, nil
				}
			}
		}
	}

	return &v1.ExportTraceServiceResponse{}, nil
}

// writeFile takes the span info and writes it out to a json file in the
// tid/sid/span.json and tid/sid/il.json files.
func (cs *cliServer) writeFile(span CliEvent, events []CliEvent) {
	// create trace directory
	outpath := filepath.Join(serverConf.outDir, span.TraceID)
	os.Mkdir(outpath, 0755) // ignore errors for now

	// create span directory
	outpath = filepath.Join(outpath, span.SpanID)
	os.Mkdir(outpath, 0755) // ignore errors for now

	// write span to file
	sjs, _ := json.Marshal(span)
	spanfile := filepath.Join(outpath, "span.json")
	err := os.WriteFile(spanfile, sjs, 0644)
	if err != nil {
		log.Fatalf("could not write span json to %q: %s", spanfile, err)
	}

	// only write events out if there is at least one
	if len(events) > 0 {
		ejs, _ := json.Marshal(events)
		eventsfile := filepath.Join(outpath, "events.json")
		err = os.WriteFile(eventsfile, ejs, 0644)
		if err != nil {
			log.Fatalf("could not write span events json to %q: %s", eventsfile, err)
		}
	}

	if serverConf.verbose {
		log.Printf("[%d] wrote trace id %s span id %s to %s\n", cs.spansSeen, span.TraceID, span.SpanID, spanfile)
	}
}

func doServer(cmd *cobra.Command, args []string) {
	listener, err := net.Listen("tcp", "127.0.0.1:4317")
	if err != nil {
		log.Fatalf("failed to listen: %s", err)
	}

	cs := cliServer{stopper: make(chan bool)}
	gs := grpc.NewServer()
	v1.RegisterTraceServiceServer(gs, &cs)

	// stops the grpc server after timeout
	go func() {
		time.Sleep(time.Duration(serverConf.timeout) * time.Second)
		cs.stopper <- true
	}()

	// single place to stop the server, used by timeout and max-spans
	go func() {
		<-cs.stopper
		gs.Stop()
	}()

	if err := gs.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
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
}

// newCliEventFromSpanEvent takes a span event, span, and ils and returns an event
// with all the span event info filled in
func newCliEventFromSpanEvent(se *tracepb.Span_Event, span *tracepb.Span, ils *tracepb.InstrumentationLibrarySpans) CliEvent {
	// start with the span, rewrite it for the event
	e := newCliEventFromSpan(span, ils)
	e.Parent = hex.EncodeToString(span.GetSpanId())
	e.Start = time.Unix(0, int64(se.GetTimeUnixNano()))
	e.End = time.Unix(0, int64(se.GetTimeUnixNano()))
	e.ElapsedMs = int64(se.GetTimeUnixNano()-span.GetStartTimeUnixNano()) / 1000000
	e.Name = se.GetName()
	e.Attributes = make(map[string]string) // overwrite the one from the span

	for _, attr := range se.GetAttributes() {
		e.Attributes[attr.GetKey()] = attr.Value.String()
	}

	return e
}

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
