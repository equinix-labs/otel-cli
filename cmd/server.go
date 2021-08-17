package cmd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	v1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"

	"google.golang.org/grpc"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "run a simple OTLP server",
	Long: `Run otel-cli as an OTLP server that can write to JSON files, print
that data to JSON, or just drop all the data.

# run otel-cli as a local server and print spans to the console as a table
otel-cli server --pterm

# write every span and event to json files in the specified directory
outdir=$(mktemp -d)
otel-cli server --json-out $outdir

# do that, but exit after 4 spans or a 30 second timeout, whichever comes first
otel-cli server --json-out $outdir --max-spans 4 --timeout 30
`,
	Run: doServer,
}

// serverConf holds the command-line configured settings for otel-cli server
var serverConf struct {
	listenAddr string
	outDir     string
	pterm      bool
	maxSpans   int
	timeout    int
	verbose    bool
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().StringVar(&serverConf.listenAddr, "listen", "127.0.0.1:4317", "the IP:PORT pair to listen on for OTLP/gRPC connections")
	serverCmd.Flags().StringVar(&serverConf.outDir, "json-out", "", "write spans to json in the specified directory")
	serverCmd.Flags().IntVar(&serverConf.maxSpans, "max-spans", 0, "exit the server after this many spans come in")
	serverCmd.Flags().IntVar(&serverConf.timeout, "timeout", 0, "exit the server after timeout seconds")
	serverCmd.Flags().BoolVar(&serverConf.verbose, "verbose", false, "print a log every time a span comes in")
	serverCmd.Flags().BoolVar(&serverConf.pterm, "pterm", false, "draw spans in the console with pterm")
}

// doServer implements the cobra command for otel-cli server.
func doServer(cmd *cobra.Command, args []string) {
	listener, err := net.Listen("tcp", serverConf.listenAddr)
	if err != nil {
		log.Fatalf("failed to listen: %s", err)
	}

	cs := cliServer{stopper: make(chan bool)}
	if serverConf.pterm {
		// TODO: implement a limit on this, drop old spans, etc.
		cs.events = []CliEvent{}

		area, err := pterm.DefaultArea.Start()
		if err != nil {
			log.Fatalf("failed to set up terminal for rendering: %s", err)
		}
		cs.area = area
	}
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
		if serverConf.pterm {
			cs.area.Stop()
		}
		gs.Stop()
	}()

	if err := gs.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}

// cliServer is a gRPC/OTLP server handle.
type cliServer struct {
	spansSeen int
	stopper   chan bool
	events    CliEventList // a queue for pterm drawing mode
	area      *pterm.AreaPrinter
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

				if serverConf.pterm {
					cs.drawPterm(ces, events)
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

// writeFile takes the spans and events and writes them out to json files in the
// tid/sid/span.json and tid/sid/events.json files.
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

// drawPterm takes the given span and events, appends them to the in-memory
// event list, sorts that, then prints it as a pterm table.
func (cs *cliServer) drawPterm(span CliEvent, events []CliEvent) {
	// append the span and its events to allocate space, might reorder
	cs.events = append(cs.events, span)
	cs.events = append(cs.events, events...)
	sort.Sort(cs.events)

	td := pterm.TableData{
		{"Trace ID", "Span ID", "Parent", "Name", "Kind", "Start", "End", "Elapsed"},
	}

	top := cs.events[0] // for calculating time offsets
	for _, e := range cs.events {
		// if the trace id changes, reset the top event used to calculate offsets
		if e.TraceID != top.TraceID {
			// make sure we have the youngest possible (expensive but whatever)
			// TODO: figure out how events are even getting inserted before a span
			top = e
			for _, te := range cs.events {
				if te.TraceID == top.TraceID && te.nanos < top.nanos {
					log.Println("SWITCHED YER TOP")
					top = te
					break
				}
			}
		}

		var startOffset, endOffset string
		if e.Kind == "event" {
			e.TraceID = ""
			e.SpanID = ""
			startOffset = strconv.FormatInt(e.Start.Sub(top.Start).Milliseconds(), 10)
		} else {
			so := e.Start.Sub(top.Start).Milliseconds()
			startOffset = strconv.FormatInt(so, 10)
			eo := e.End.Sub(top.Start).Milliseconds()
			endOffset = strconv.FormatInt(eo, 10)
		}

		td = append(td, []string{
			e.TraceID,
			e.SpanID,
			e.Parent,
			e.Name,
			e.Kind,
			startOffset,
			endOffset,
			strconv.FormatInt(e.ElapsedMs, 10),
		})
	}

	cs.area.Update(pterm.DefaultTable.WithHasHeader().WithData(td).Srender())
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
