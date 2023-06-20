package otelcli

import (
	"encoding/hex"
	"log"
	"math"
	"sort"
	"strconv"

	"github.com/equinix-labs/otel-cli/otlpclient"
	"github.com/equinix-labs/otel-cli/otlpserver"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

var tuiServer struct {
	lines  SpanEventUnionList
	traces map[string]*tracepb.Span // for looking up top span of trace by trace id
	area   *pterm.AreaPrinter
}

func serverTuiCmd(config *otlpclient.Config) *cobra.Command {
	cmd := cobra.Command{
		Use:   "tui",
		Short: "display spans in a terminal UI",
		Long: `Run otel-cli as an OTLP server with a terminal UI that displays traces.
	
	# run otel-cli as a local server and print spans to the console as a table
	otel-cli server tui`,
		Run: doServerTui,
	}

	addCommonParams(&cmd, config)
	return &cmd
}

// doServerTui implements the 'otel-cli server tui' subcommand.
func doServerTui(cmd *cobra.Command, args []string) {
	config := getConfig(cmd.Context())
	area, err := pterm.DefaultArea.Start()
	if err != nil {
		log.Fatalf("failed to set up terminal for rendering: %s", err)
	}
	tuiServer.area = area

	tuiServer.lines = []SpanEventUnion{}
	tuiServer.traces = make(map[string]*tracepb.Span)

	stop := func(otlpserver.OtlpServer) {
		tuiServer.area.Stop()
	}

	runServer(config, renderTui, stop)
}

// renderTui takes the given span and events, appends them to the in-memory
// event list, sorts that, then prints it as a pterm table.
func renderTui(span *tracepb.Span, events []*tracepb.Span_Event, rss *tracepb.ResourceSpans, meta map[string]string) bool {
	spanTraceId := hex.EncodeToString(span.TraceId)
	if _, ok := tuiServer.traces[spanTraceId]; !ok {
		tuiServer.traces[spanTraceId] = span
	}

	tuiServer.lines = append(tuiServer.lines, SpanEventUnion{Span: span})
	for _, e := range events {
		tuiServer.lines = append(tuiServer.lines, SpanEventUnion{Span: span, Event: e})
	}
	sort.Sort(tuiServer.lines)
	trimTuiEvents()

	td := pterm.TableData{
		{"Trace ID", "Span ID", "Parent", "Name", "Kind", "Start", "End", "Elapsed"},
	}

	for _, line := range tuiServer.lines {
		var traceId, spanId, parent, name, kind string
		var startOffset, endOffset, elapsed int64
		if line.IsSpan() {
			name = line.Span.Name
			kind = otlpclient.SpanKindIntToString(line.Span.GetKind())
			traceId = line.TraceIdString()
			spanId = line.SpanIdString()

			if tspan, ok := tuiServer.traces[traceId]; ok {
				startOffset = roundedDelta(line.Span.StartTimeUnixNano, tspan.StartTimeUnixNano)
				endOffset = roundedDelta(line.Span.EndTimeUnixNano, tspan.StartTimeUnixNano)
			} else {
				endOffset = roundedDelta(line.Span.EndTimeUnixNano, line.Span.StartTimeUnixNano)
			}

			if len(line.Span.ParentSpanId) > 0 {
				traceId = "" // hide it after printing the first trace id
				parent = hex.EncodeToString(line.Span.ParentSpanId)
			}

			elapsed = endOffset - startOffset
		} else { // span events
			name = line.Event.Name
			kind = "event"
			traceId = "" // hide ids on events to make screen less busy
			parent = line.SpanIdString()
			if tspan, ok := tuiServer.traces[traceId]; ok {
				startOffset = roundedDelta(line.Event.TimeUnixNano, tspan.StartTimeUnixNano)
			} else {
				startOffset = roundedDelta(line.Event.TimeUnixNano, line.Span.StartTimeUnixNano)
			}
			endOffset = startOffset
			elapsed = 0
		}

		td = append(td, []string{
			traceId,
			spanId,
			parent,
			name,
			kind,
			strconv.FormatInt(startOffset, 10),
			strconv.FormatInt(endOffset, 10),
			strconv.FormatInt(elapsed, 10),
		})
	}

	tuiServer.area.Update(pterm.DefaultTable.WithHasHeader().WithData(td).Srender())
	return false // keep running until user hits ctrl-c
}

// roundedDelta takes to uint64 nanos values, cuts them down to milliseconds,
// takes the delta (absolute value, so any order is fine), and returns an int64
// of ms between the values.
func roundedDelta(ts1, ts2 uint64) int64 {
	deltaMs := math.Abs(float64(ts1/1000000) - float64(ts2/1000000))
	rounded := math.Round(deltaMs)
	return int64(rounded)
}

// trimEvents looks to see if there's room on the screen for the number of incoming
// events and removes the oldest traces until there's room
// TODO: how to hand really huge traces that would scroll off the screen entirely?
func trimTuiEvents() {
	maxRows := pterm.GetTerminalHeight() // TODO: allow override of this?

	if len(tuiServer.lines) == 0 || len(tuiServer.lines) < maxRows {
		return // plenty of room, nothing to do
	}

	end := len(tuiServer.lines) - 1              // should never happen but default to all
	need := (len(tuiServer.lines) - maxRows) + 2 // trim at least this many
	tid := tuiServer.lines[0].TraceIdString()    // we always remove the whole trace
	for i, v := range tuiServer.lines {
		if v.TraceIdString() == tid {
			end = i
		} else {
			if end+1 < need {
				// trace id changed, advance the trim point, and change trace ids
				tid = v.TraceIdString()
				end = i
			} else {
				break // made enough room, we can quit early
			}
		}
	}

	// might need to realloc to not leak memory here?
	tuiServer.lines = tuiServer.lines[end:]
}

// SpanEventUnion is for server_tui so it can sort spans and events together
// by timestamp.
type SpanEventUnion struct {
	Span  *tracepb.Span
	Event *tracepb.Span_Event
}

func (seu *SpanEventUnion) TraceIdString() string { return hex.EncodeToString(seu.Span.TraceId) }
func (seu *SpanEventUnion) SpanIdString() string  { return hex.EncodeToString(seu.Span.SpanId) }

func (seu *SpanEventUnion) UnixNanos() uint64 {
	if seu.IsSpan() {
		return seu.Span.StartTimeUnixNano
	} else {
		return seu.Event.TimeUnixNano
	}
}

// IsSpan returns true if this union is for an event. Span is always populated
// but Event is only populated for events.
func (seu *SpanEventUnion) IsSpan() bool { return seu.Event == nil }

// SpanEventUnionList is a sortable list of SpanEventUnion, sorted on timestamp.
type SpanEventUnionList []SpanEventUnion

func (sl SpanEventUnionList) Len() int           { return len(sl) }
func (sl SpanEventUnionList) Swap(i, j int)      { sl[i], sl[j] = sl[j], sl[i] }
func (sl SpanEventUnionList) Less(i, j int) bool { return sl[i].UnixNanos() < sl[j].UnixNanos() }
