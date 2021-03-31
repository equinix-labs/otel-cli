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

var spanBgSockdir string
var spanBgTimeout int

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
	spanCmd.AddCommand(spanBgCmd)
	spanBgCmd.Flags().SortFlags = false
	// it seems like the socket should be required for background but it's
	// only necessary for adding events to the span. it should be fine to
	// start a background span at the top of a script then let it fall off
	// at the end to get an easy span
	spanBgCmd.Flags().StringVar(&spanBgSockdir, "sockdir", "", "a directory where a socket can be placed safely")
	spanBgCmd.Flags().IntVar(&spanBgTimeout, "timeout", 10, "how long the background server should run before timeout")
}

// spanBgSockfile returns the full filename for the socket file under the
// provided background socket directory provided by the user.
func spanBgSockfile() string {
	return path.Join(spanBgSockdir, spanBgSockfilename)
}

func doSpanBackground(cmd *cobra.Command, args []string) {
	startup := time.Now()
	ctx, span, shutdown := startSpan() // from span.go
	defer shutdown()

	bgs := createBgServer(spanBgSockfile(), span)

	// set up signal handlers to cleanly exit on SIGINT/SIGTERM etc
	signals := make(chan os.Signal)
	signal.Notify(signals, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signals
		bgs.Shutdown()
	}()

	// start the timeout goroutine, this is a little late but the server
	// has to be up for this to make much sense
	go func() {
		time.Sleep(time.Second * time.Duration(spanBgTimeout))

		uptime := time.Since(startup).Milliseconds()
		attrs := trace.WithAttributes([]attribute.KeyValue{
			{Key: attribute.Key("uptime.milliseconds"), Value: attribute.Int64Value(uptime)},
			{Key: attribute.Key("timeout.seconds"), Value: attribute.IntValue(spanBgTimeout)},
		}...)
		span.AddEvent("timeout", attrs)
		bgs.Shutdown()
	}()

	// will block until bgs.Shutdown()
	bgs.Run()

	endSpan(span)
	finishOtelCliSpan(ctx, span)
}
