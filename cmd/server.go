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

type cliServer struct {
	spansSeen int
	stopper   chan bool
	v1.UnimplementedTraceServiceServer
}

func (cs *cliServer) Export(ctx context.Context, req *v1.ExportTraceServiceRequest) (*v1.ExportTraceServiceResponse, error) {
	rss := req.GetResourceSpans()
	for _, resource := range rss {
		ilSpans := resource.GetInstrumentationLibrarySpans()
		for _, ilSpan := range ilSpans {
			for _, span := range ilSpan.GetSpans() {
				tid := hex.EncodeToString(span.TraceId)
				sid := hex.EncodeToString(span.SpanId)

				cs.spansSeen++ // count spans for exiting on --max-spans
				if serverConf.maxSpans > 0 && cs.spansSeen >= serverConf.maxSpans {
					cs.stopper <- true // shus the server down
					return &v1.ExportTraceServiceResponse{}, nil
				}

				cs.writeFile(tid, sid, ilSpan, span)
			}
		}
	}

	return &v1.ExportTraceServiceResponse{}, nil
}

// writeFile takes the span info and writes it out to a json file in the
// tid/sid/span.json and tid/sid/il.json files.
func (cs *cliServer) writeFile(tid, sid string, ilSpans *tracepb.InstrumentationLibrarySpans, span *tracepb.Span) {
	// create trace directory
	outpath := filepath.Join(serverConf.outDir, tid)
	os.Mkdir(outpath, 0755) // ignore errors for now

	// create span directory
	outpath = filepath.Join(outpath, sid)
	os.Mkdir(outpath, 0755) // ignore errors for now

	// write instrumentation library (no idea why this is separate)
	ijs, _ := json.Marshal(ilSpans.InstrumentationLibrary)
	ilfile := filepath.Join(outpath, "il.json")
	err := os.WriteFile(ilfile, ijs, 0644)
	if err != nil {
		log.Fatalf("could not write to %q: %s", ilfile, err)
	}

	// write span to file
	sjs, _ := json.Marshal(span)
	spanfile := filepath.Join(outpath, "span.json")
	err = os.WriteFile(spanfile, sjs, 0644)
	if err != nil {
		log.Fatalf("could not write to %q: %s", spanfile, err)
	}

	if serverConf.verbose {
		log.Printf("[%d] wrote trace id %s span id %s to %s\n", cs.spansSeen, tid, sid, spanfile)
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
