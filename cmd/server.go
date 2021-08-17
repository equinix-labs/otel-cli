package cmd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	v1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "run a simple OTLP server",
	Long: `
outdir=$(mktemp -d)
otel-cli server -d $outdir`,
	Run: doServer,
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().StringVarP(&serverOutDir, "outdir", "d", "", "write spans to json in the specified directory")
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
				js, _ := json.Marshal(span)

				outpath := filepath.Join(serverOutDir, tid)
				os.Mkdir(outpath, 0755) // ignore errors for now

				outfile := filepath.Join(outpath, sid+".json")
				err := os.WriteFile(outfile, js, 0644)
				if err != nil {
					log.Fatalf("could not write to %q: %s", outfile, err)
				}

				log.Printf("wrote trace id %s span id %s to %s\n", tid, sid, outfile)
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

	if err := gs.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}
