package cmd

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// BgSpan is what is returned to all RPC clients and its methods are exported.
type BgSpan struct {
	TraceID     string `json:"trace_id"`
	SpanID      string `json:"span_id"`
	Traceparent string `json:"traceparent"`
	Error       string `json:"error"`
	span        trace.Span
	shutdown    func()
}

// BgSpanEvent is a span event that the client will send.
type BgSpanEvent struct {
	Name       string `json:"name"`
	Timestamp  string `json:"timestamp"`
	Attributes map[string]string
}

// BgEnd is an empty struct that can be sent to call End().
type BgEnd struct{}

// AddEvent takes a BgSpanEvent from the client and attaches an event to the span.
func (bs BgSpan) AddEvent(bse *BgSpanEvent, reply *BgSpan) error {
	reply.TraceID = bs.TraceID
	reply.SpanID = bs.SpanID
	ctx := trace.ContextWithSpan(context.Background(), bs.span)
	reply.Traceparent = getTraceparent(ctx)

	ts, err := time.Parse(time.RFC3339Nano, bse.Timestamp)
	if err != nil {
		reply.Error = fmt.Sprintf("%s", err)
		return err
	}

	otelOpts := []trace.EventOption{
		trace.WithTimestamp(ts),
		// use the cli helper since it already does string map to otel
		trace.WithAttributes(cliAttrsToOtel(bse.Attributes)...),
	}

	bs.span.AddEvent(bse.Name, otelOpts...)

	return nil
}

// End takes a BgEnd (empty) struct, replies with the usual trace info, then
// ends the span end exits the background process.
func (bs BgSpan) End(in *BgEnd, reply *BgSpan) error {
	// TODO: maybe accept an end timestamp?
	endSpan(bs.span)
	// running the shutdown as a goroutine prevents the client from getting an
	// error here when the server gets closed. defer didn't do the trick.
	go bs.shutdown()
	return nil
}

// bgServer is a handle for a span background server.
type bgServer struct {
	sockfile string
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
		quit:     make(chan struct{}),
	}

	// TODO: be safer?
	if err = os.RemoveAll(sockfile); err != nil {
		log.Fatalf("failed while cleaning up for socket file '%s': %s", sockfile, err)
	}

	bgspan := BgSpan{
		TraceID:  span.SpanContext().TraceID().String(),
		SpanID:   span.SpanContext().SpanID().String(),
		span:     span,
		shutdown: func() { bgs.Shutdown() },
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

// createBgClient sets up a client connection to the unix socket jsonrpc server
// and returns the rpc client handle and a shutdown function that should be
// deferred.
func createBgClient() (*rpc.Client, func()) {
	sockfile := spanBgSockfile()
	started := time.Now()

	// wait for the socket file to show up, polling every 25ms until it does or timeout
	for {
		_, err := os.Stat(sockfile)
		if os.IsNotExist(err) {
			time.Sleep(time.Millisecond * 25) // sleep 25ms between checks
		} else if err != nil {
			log.Fatalf("failed to stat file '%s': %s", sockfile, err)
		} else {
			break
		}

		to := parseCliTimeout()
		if to > 0 && time.Since(started) > to {
			log.Fatalf("timeout after %s while waiting for span background socket '%s' to appear", config.Timeout, sockfile)
		}
	}

	sock := net.UnixAddr{Name: sockfile, Net: "unix"}
	conn, err := net.DialUnix(sock.Net, nil, &sock)
	if err != nil {
		log.Fatalf("unable to connect to span background server at '%s': %s", spanBgSockdir, err)
	}

	return jsonrpc.NewClient(conn), func() { conn.Close() }
}
