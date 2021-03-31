package cmd

import (
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"
)

//const bgSocketFile = "background-server.sock"

// BgSpan is what is returned to all RPC clients and its methods are exported.
type BgSpan struct {
	TraceID string `json:"trace_id"`
	SpanID  string `json:"span_id"`
	Error   string `json:"error"`
	span    trace.Span
}

// BgSpanEvent is a span event that the client will send.
type BgSpanEvent struct {
	Name       string `json:"name"`
	Timestamp  string `json:"timestamp"`
	Attributes map[string]string
}

// Ping is an exported RPC that takes any string and returns BgSpan.
func (bs BgSpan) Ping(arg *string, reply *BgSpan) error {
	reply.TraceID = bs.TraceID
	reply.SpanID = bs.SpanID
	return nil
}

// AddEvent takes a BgSpanEvent from the client and attaches an event to the span.
func (bs BgSpan) AddEvent(bse *BgSpanEvent, reply *BgSpan) error {
	reply.TraceID = bs.TraceID
	reply.SpanID = bs.SpanID

	ts, err := time.Parse(time.RFC3339Nano, bse.Timestamp)
	if err != nil {
		log.Fatalf("failed to parse timestamp field in request: %s", err)
	}

	otelOpts := []trace.EventOption{
		trace.WithTimestamp(ts),
		// use the cli helper since it already does string map to otel
		trace.WithAttributes(cliAttrsToOtel(bse.Attributes)...),
	}

	bs.span.AddEvent(bse.Name, otelOpts...)

	return nil
}

// bgServer is a handle for a span background server.
type bgServer struct {
	sockfile string
	span     trace.Span
	listener net.Listener
	quit     chan struct{}
	wg       sync.WaitGroup
}

// createBgServer opens a new span background server on a unix socket and
// returns with the server ready to go. Not expected to block.
func createBgServer(sockfile string, span trace.Span) *bgServer {
	var err error

	bgs := bgServer{
		sockfile: sockfile,
		span:     span,
		quit:     make(chan struct{}),
	}

	// TODO: be safer?
	if err = os.RemoveAll(sockfile); err != nil {
		log.Fatalf("failed while cleaning up for socket file '%s': %s", sockfile, err)
	}

	bgspan := BgSpan{
		TraceID: span.SpanContext().TraceID().String(),
		SpanID:  span.SpanContext().SpanID().String(),
	}
	// makes methods on BgSpan available over RPC
	rpc.Register(&bgspan)

	bgs.listener, err = net.Listen("unix", sockfile)
	if err != nil {
		log.Fatalf("unable to listen on unix socket '%s': %s", sockfile, err)
	}

	bgs.wg.Add(1) // cleanup will block until this is done

	return &bgs
}

// Run will block until shutdown, accepting connections and processing them.
func (bgs *bgServer) Run() {
	// TODO: add controls to exit loop
	for {
		conn, err := bgs.listener.Accept()
		if err != nil {
			select {
			case <-bgs.quit: // quitting gracefully
				log.Println("quit channel closed, returning")
				return
			default:
				log.Fatalf("error while accepting connection: %s", err)
			}
		}

		bgs.wg.Add(1)
		go func() {
			defer conn.Close()
			jsonrpc.ServeConn(conn)
			bgs.wg.Done()
		}()
	}
}

// Shutdown does a controlled shutdown of the background server. Blocks until
// the server is turned down cleanly and it's safe to exit.
func (bgs *bgServer) Shutdown() {
	close(bgs.quit)
	bgs.listener.Close()
	bgs.wg.Wait()
}
