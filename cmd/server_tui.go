package cmd

import (
	"log"
	"sort"
	"strconv"

	"github.com/equinix-labs/otel-cli/otlpserver"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var tuiServer struct {
	events otlpserver.CliEventList
	area   *pterm.AreaPrinter
}

var serverTuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "display spans in a terminal UI",
	Long: `Run otel-cli as an OTLP server with a terminal UI that displays traces.
	
	# run otel-cli as a local server and print spans to the console as a table
	otel-cli server tui`,
	Run: doServerTui,
}

func init() {
	serverCmd.AddCommand(serverTuiCmd)
	addCommonParams(serverTuiCmd)
}

// doServerTui implements the 'otel-cli server tui' subcommand.
func doServerTui(cmd *cobra.Command, args []string) {
	area, err := pterm.DefaultArea.Start()
	if err != nil {
		log.Fatalf("failed to set up terminal for rendering: %s", err)
	}
	tuiServer.area = area

	tuiServer.events = []otlpserver.CliEvent{}

	stop := func(*otlpserver.Server) {
		tuiServer.area.Stop()
	}

	cs := otlpserver.NewServer(renderTui, stop)

	// unlike the rest of otel-cli, server should default to localhost:4317
	if config.Endpoint == "" {
		config.Endpoint = defaultOtlpEndpoint
	}
	cs.ServeGPRC(config.Endpoint)
}

// renderTui takes the given span and events, appends them to the in-memory
// event list, sorts that, then prints it as a pterm table.
func renderTui(span otlpserver.CliEvent, events otlpserver.CliEventList) bool {
	tuiServer.events = append(tuiServer.events, span)
	tuiServer.events = append(tuiServer.events, events...)
	sort.Sort(tuiServer.events)
	trimTuiEvents()

	td := pterm.TableData{
		{"Trace ID", "Span ID", "Parent", "Name", "Kind", "Start", "End", "Elapsed"},
	}

	top := tuiServer.events[0] // for calculating time offsets
	for _, e := range tuiServer.events {
		// if the trace id changes, reset the top event used to calculate offsets
		if e.TraceID != top.TraceID {
			// make sure we have the youngest possible (expensive but whatever)
			// TODO: figure out how events are even getting inserted before a span
			top = e
			for _, te := range tuiServer.events {
				if te.TraceID == top.TraceID && te.Nanos < top.Nanos {
					top = te
					break
				}
			}
		}

		var startOffset, endOffset string
		if e.Kind == "event" {
			e.TraceID = "" // hide ids on events to make screen less busy
			e.SpanID = ""
			startOffset = strconv.FormatInt(e.Start.Sub(top.Start).Milliseconds(), 10)
		} else {
			if e.TraceID == top.TraceID && e.SpanID != top.SpanID {
				e.TraceID = "" // hide it after printing the first trace id
			}
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

	tuiServer.area.Update(pterm.DefaultTable.WithHasHeader().WithData(td).Srender())
	return false // keep running until user hits ctrl-c
}

// trimEvents looks to see if there's room on the screen for the number of incoming
// events and removes the oldest traces until there's room
// TODO: how to hand really huge traces that would scroll off the screen entirely?
func trimTuiEvents() {
	maxRows := pterm.GetTerminalHeight() // TODO: allow override of this?

	if len(tuiServer.events) == 0 || len(tuiServer.events) < maxRows {
		return // plenty of room, nothing to do
	}

	end := len(tuiServer.events) - 1              // should never happen but default to all
	need := (len(tuiServer.events) - maxRows) + 2 // trim at least this many
	tid := tuiServer.events[0].TraceID            // we always remove the whole trace
	for i, v := range tuiServer.events {
		if v.TraceID == tid {
			end = i
		} else {
			if end+1 < need {
				// trace id changed, advance the trim point, and change trace ids
				tid = v.TraceID
				end = i
			} else {
				break // made enough room, we can quit early
			}
		}
	}

	// might need to realloc to not leak memory here?
	tuiServer.events = tuiServer.events[end:]
}
