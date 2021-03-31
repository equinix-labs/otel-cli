package cmd

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

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
		--system-name "my-long-script.sh" \
		--span-name "run the script" \
		--attrs "os.kernel=$(uname -r)" \
		--timeout 60 \
		--sockdir $socket_dir & # <-- notice the &
	
	otel-cli span event \
		--sockdir $socket_dir \
		--event-name "something interesting happened!" \
		--attrs "foo=bar"
`,
	Run: doSpanBackground,
}

func init() {
	spanCmd.AddCommand(spanBgCmd)
	spanBgCmd.Flags().SortFlags = false
	spanBgCmd.Flags().StringVar(&spanBgSockdir, "sockdir", "", "a directory where a socket can be placed safely")
	spanBgCmd.Flags().IntVar(&spanBgTimeout, "timeout", 10, "how long the background server should run before timeout")
}

func doSpanBackground(cmd *cobra.Command, args []string) {
	ctx, span, shutdown := startSpan() // from span.go
	defer shutdown()

	bgs := createBgServer(spanBgSockdir, span)

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
		bgs.Shutdown()
	}()

	// will block until bgs.Shutdown()
	bgs.Run()

	endSpan(span)
	finishOtelCliSpan(ctx, span)
}
