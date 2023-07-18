package otelcli

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/equinix-labs/otel-cli/otlpserver"
	"github.com/spf13/cobra"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// jsonSvr holds the command-line configured settings for otel-cli server json
var jsonSvr struct {
	outDir    string
	stdout    bool
	maxSpans  int
	spansSeen int
}

func serverJsonCmd(config *Config) *cobra.Command {
	cmd := cobra.Command{
		Use:   "json",
		Short: "write spans to json or stdout",
		Long:  "",
		Run:   doServerJson,
	}

	addCommonParams(&cmd, config)
	cmd.Flags().StringVar(&jsonSvr.outDir, "dir", "", "write spans to json in the specified directory")
	cmd.Flags().BoolVar(&jsonSvr.stdout, "stdout", false, "write span jsons to stdout")
	cmd.Flags().IntVar(&jsonSvr.maxSpans, "max-spans", 0, "exit the server after this many spans come in")

	return &cmd
}

func doServerJson(cmd *cobra.Command, args []string) {
	config := getConfig(cmd.Context())
	stop := func(otlpserver.OtlpServer) {}
	cs := otlpserver.NewGrpcServer(renderJson, stop)

	// stops the grpc server after timeout
	timeout := config.ParseCliTimeout()
	if timeout > 0 {
		go func() {
			time.Sleep(timeout)
			cs.Stop()
		}()
	}

	runServer(config, renderJson, stop)
}

// writeFile takes the spans and events and writes them out to json files in the
// tid/sid/span.json and tid/sid/events.json files.
func renderJson(ctx context.Context, span *tracepb.Span, events []*tracepb.Span_Event, ss *tracepb.ResourceSpans, headers map[string]string, meta map[string]string) bool {
	jsonSvr.spansSeen++ // count spans for exiting on --max-spans

	// TODO: check for existence of outdir and error when it doesn't exist
	var outpath string
	if jsonSvr.outDir != "" {
		// create trace directory
		outpath = filepath.Join(jsonSvr.outDir, hex.EncodeToString(span.TraceId))
		os.Mkdir(outpath, 0755) // ignore errors for now

		// create span directory
		outpath = filepath.Join(outpath, hex.EncodeToString(span.SpanId))
		os.Mkdir(outpath, 0755) // ignore errors for now
	}

	// write span to file
	// TODO: if a span comes in twice should we continue to overwrite span.json
	// or attempt some kind of merge? (e.g. of attributes)
	sjs, err := json.Marshal(span)
	if err != nil {
		log.Fatalf("failed to marshal span to json: %s", err)
	}

	// write the span to /path/tid/sid/span.json
	writeJson(outpath, "span.json", sjs)

	// only write events out if there is at least one
	for i, e := range events {
		ejs, err := json.Marshal(e)
		if err != nil {
			log.Fatalf("failed to marshal span event to json: %s", err)
		}

		// write events to /path/tid/sid/event-%d.json
		// TODO: ordering might be a problem if people rely on it...
		filename := "event-" + strconv.Itoa(i) + ".json"
		writeJson(outpath, filename, ejs)
	}

	if jsonSvr.maxSpans > 0 && jsonSvr.spansSeen >= jsonSvr.maxSpans {
		return true // will cause the server loop to exit
	}

	return false
}

// writeJson takes a directory path, a filename, and json. When the path is not empty
// string the json is written to path/filename. If --stdout was specified the json will
// be printed as a line to stdout.
func writeJson(path, filename string, js []byte) {
	if path != "" {
		spanfile := filepath.Join(path, filename)
		err := os.WriteFile(spanfile, js, 0644)
		if err != nil {
			log.Fatalf("could not write to file %q: %s", spanfile, err)
		}
	}

	if jsonSvr.stdout {
		os.Stdout.Write(js)
		os.Stdout.WriteString("\n")
	}
}
