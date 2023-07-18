package otelcli

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"path"
	"sync"
	"time"

	"github.com/equinix-labs/otel-cli/otlpclient"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// BgSpan is what is returned to all RPC clients and its methods are exported.
type BgSpan struct {
	TraceID     string `json:"trace_id"`
	SpanID      string `json:"span_id"`
	Traceparent string `json:"traceparent"`
	Error       string `json:"error"`
	config      otlpclient.Config
	span        *tracepb.Span
	shutdown    func()
}

// BgSpanEvent is a span event that the client will send.
type BgSpanEvent struct {
	Name       string `json:"name"`
	Timestamp  string `json:"timestamp"`
	Attributes map[string]string
}

// BgEnd is an empty struct that can be sent to call End().
type BgEnd struct {
	StatusCode string `json:"status_code"`
	StatusDesc string `json:"status_description"`
}

// AddEvent takes a BgSpanEvent from the client and attaches an event to the span.
func (bs BgSpan) AddEvent(bse *BgSpanEvent, reply *BgSpan) error {
	reply.TraceID = hex.EncodeToString(bs.span.TraceId)
	reply.SpanID = hex.EncodeToString(bs.span.SpanId)
	reply.Traceparent = otlpclient.TraceparentFromProtobufSpan(bs.config, bs.span).Encode()

	ts, err := time.Parse(time.RFC3339Nano, bse.Timestamp)
	if err != nil {
		reply.Error = fmt.Sprintf("%s", err)
		return err
	}

	event := otlpclient.NewProtobufSpanEvent()
	event.Name = bse.Name
	event.TimeUnixNano = uint64(ts.UnixNano())
	event.Attributes = otlpclient.StringMapAttrsToProtobuf(bse.Attributes)

	bs.span.Events = append(bs.span.Events, event)

	return nil
}

// Wait is a no-op RPC for validating the background server is up and running.
func (bs BgSpan) Wait(in, reply *struct{}) error {
	return nil
}

// End takes a BgEnd (empty) struct, replies with the usual trace info, then
// ends the span end exits the background process.
func (bs BgSpan) End(in *BgEnd, reply *BgSpan) error {
	// handle --status-code and --status-description args to span end
	c := bs.config.WithStatusCode(in.StatusCode).WithStatusDescription(in.StatusDesc)
	otlpclient.SetSpanStatus(bs.span, c.StatusCode, c.StatusDescription)

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
	config   otlpclient.Config
}

// createBgServer opens a new span background server on a unix socket and
// returns with the server ready to go. Not expected to block.
func createBgServer(ctx context.Context, sockfile string, span *tracepb.Span) *bgServer {
	var err error
	config := getConfig(ctx)

	bgs := bgServer{
		sockfile: sockfile,
		quit:     make(chan struct{}),
		config:   config,
	}

	// TODO: be safer?
	if err = os.RemoveAll(sockfile); err != nil {
		config.SoftFail("failed while cleaning up for socket file '%s': %s", sockfile, err)
	}

	bgspan := BgSpan{
		TraceID:  hex.EncodeToString(span.TraceId),
		SpanID:   hex.EncodeToString(span.SpanId),
		config:   config,
		span:     span,
		shutdown: func() { bgs.Shutdown() },
	}
	// makes methods on BgSpan available over RPC
	rpc.Register(&bgspan)

	bgs.listener, err = net.Listen("unix", sockfile)
	if err != nil {
		config.SoftFail("unable to listen on unix socket '%s': %s", sockfile, err)
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
				bgs.config.SoftFail("error while accepting connection: %s", err)
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
func createBgClient(config otlpclient.Config) (*rpc.Client, func()) {
	sockfile := path.Join(config.BackgroundSockdir, spanBgSockfilename)
	started := time.Now()
	timeout := config.ParseCliTimeout()

	// wait for the socket file to show up, polling every 25ms until it does or timeout
	for {
		_, err := os.Stat(sockfile)
		if os.IsNotExist(err) {
			time.Sleep(time.Millisecond * 25) // sleep 25ms between checks
		} else if err != nil {
			config.SoftFail("failed to stat file '%s': %s", sockfile, err)
		} else {
			break
		}

		if timeout > 0 && time.Since(started) > timeout {
			config.SoftFail("timeout after %s while waiting for span background socket '%s' to appear", config.Timeout, sockfile)
		}
	}

	sock := net.UnixAddr{Name: sockfile, Net: "unix"}
	conn, err := net.DialUnix(sock.Net, nil, &sock)
	if err != nil {
		config.SoftFail("unable to connect to span background server at '%s': %s", config.BackgroundSockdir, err)
	}

	return jsonrpc.NewClient(conn), func() { conn.Close() }
}
