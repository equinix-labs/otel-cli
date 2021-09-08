package cmd

import (
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const spanBgSockfilename = "otel-cli-background.sock"
const spanBgParentPollMs = 10 // check parent pid every 10ms

var spanBgSockdir string
var spanBgStarted time.Time // for measuring uptime

// spanBgCmd represents the span background command
var spanBgCmd = &cobra.Command{
	Use:   "background",
	Short: "create background span handler",
	Long: `Creates a background span handler that listens on a Unix socket
so you can add events to it. The span is closed when the process exits from
timeout, (catchable) signals, or deliberate exit.

    socket_dir=$(mktemp -d)
	otel-cli span background \
		--system "my-long-script.sh" \
		--name "run the script" \
		--attrs "os.kernel=$(uname -r)" \
		--timeout 60 \
		--sockdir $socket_dir & # <-- notice the &
	
	otel-cli span event \
		--sockdir $socket_dir \
		--name "something interesting happened!" \
		--attrs "foo=bar"
`,
	Run: doSpanBackground,
}

func init() {
	// mark the time when the process started for putting uptime in attributes
	spanBgStarted = time.Now()

	spanCmd.AddCommand(spanBgCmd)
	spanBgCmd.Flags().SortFlags = false
	// it seems like the socket should be required for background but it's
	// only necessary for adding events to the span. it should be fine to
	// start a background span at the top of a script then let it fall off
	// at the end to get an easy span
	spanBgCmd.Flags().StringVar(&spanBgSockdir, "sockdir", "", "a directory where a socket can be placed safely")

	addCommonParams(spanBgCmd)
	addSpanParams(spanBgCmd)
	addClientParams(spanBgCmd)
}

// spanBgSockfile returns the full filename for the socket file under the
// provided background socket directory provided by the user.
func spanBgSockfile() string {
	return path.Join(spanBgSockdir, spanBgSockfilename)
}

func doSpanBackground(cmd *cobra.Command, args []string) {
	ctx, span, shutdown := startSpan() // from span.go
	defer shutdown()

	// span background is a bit different from span/exec in that it might be
	// hanging out while other spans are created, so it does the traceparent
	// propagation before the server starts, instead of after
	propagateOtelCliSpan(ctx, span, os.Stdout)

	bgs := createBgServer(spanBgSockfile(), span)

	// set up signal handlers to cleanly exit on SIGINT/SIGTERM etc
	signals := make(chan os.Signal)
	signal.Notify(signals, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signals
		bgs.Shutdown()
	}()

	// in order to exit at the end of scripts this program needs a way to know
	// when the parent is gone. the most straightforward approach that should
	// be fine on most Unix-ish operating systems is to poll getppid and exit
	// when the parent process pid changes
	// TODO: make this configurable?
	ppid := os.Getppid()
	go func() {
		for {
			time.Sleep(time.Duration(spanBgParentPollMs) * time.Millisecond)

			// check if the parent process has changed, exit when it does
			cppid := os.Getppid()
			if cppid != ppid {
				spanBgEndEvent("parent_exited", span)
				bgs.Shutdown()
			}
		}
	}()

	// start the timeout goroutine, this is a little late but the server
	// has to be up for this to make much sense
	if timeout := parseCliTimeout(); timeout > 0 {
		go func() {
			time.Sleep(timeout)
			spanBgEndEvent("timeout", span)
			bgs.Shutdown()
		}()
	}

	// will block until bgs.Shutdown()
	bgs.Run()

	endSpan(span)
}

// spanBgEndEvent adds an event with the provided name, to the provided span
// with uptime.milliseconds and timeout.seconds attributes.
func spanBgEndEvent(name string, span trace.Span) {
	uptime := time.Since(spanBgStarted)
	attrs := trace.WithAttributes([]attribute.KeyValue{
		{Key: attribute.Key("uptime.milliseconds"), Value: attribute.Int64Value(uptime.Milliseconds())},
		{Key: attribute.Key("timeout.duration"), Value: attribute.StringValue(config.Timeout)},
	}...)
	span.AddEvent(name, attrs)
}
