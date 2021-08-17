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
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().StringVar(&serverConf.outDir, "json-out", "", "write spans to json in the specified directory")
	serverCmd.Flags().IntVar(&serverConf.maxSpans, "max-spans", 0, "exit the server after this many spans come in")
	serverCmd.Flags().IntVar(&serverConf.timeout, "timeout", 0, "exit the server after timeout seconds")
}

type cliServer struct {
	v1.UnimplementedTraceServiceServer
}

func (cs cliServer) Export(ctx context.Context, req *v1.ExportTraceServiceRequest) (*v1.ExportTraceServiceResponse, error) {
	rss := req.GetResourceSpans()
	for _, resource := range rss {
		ilSpans := resource.GetInstrumentationLibrarySpans()
		for _, ilSpan := range ilSpans {
			for _, span := range ilSpan.GetSpans() {
				tid := hex.EncodeToString(span.TraceId)
				sid := hex.EncodeToString(span.SpanId)

				// create trace directory
				outpath := filepath.Join(serverConf.outDir, tid)
				os.Mkdir(outpath, 0755) // ignore errors for now

				// create span directory
				outpath = filepath.Join(outpath, sid)
				os.Mkdir(outpath, 0755) // ignore errors for now

				// write instrumentation library (no idea why this is separate)
				ijs, _ := json.Marshal(ilSpan.InstrumentationLibrary)
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

				log.Printf("wrote trace id %s span id %s to %s\n", tid, sid, spanfile)
			}
		}
	}

	return &v1.ExportTraceServiceResponse{}, nil
}

func doServer(cmd *cobra.Command, args []string) {
	listener, err := net.Listen("tcp", "127.0.0.1:4317")
	if err != nil {
		log.Fatalf("failed to listen: %s", err)
	}

	gs := grpc.NewServer()
	v1.RegisterTraceServiceServer(gs, cliServer{})

	// stops the grpc server after timeout
	go func() {
		time.Sleep(time.Duration(serverConf.timeout) * time.Second)
		gs.Stop()
	}()

	if err := gs.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}
