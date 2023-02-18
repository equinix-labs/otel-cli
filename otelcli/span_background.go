package otelcli

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

// spanBgCmd represents the span background command
var spanBgCmd = &cobra.Command{
	Use:   "background",
	Short: "create background span handler",
	Long: `Creates a background span handler that listens on a Unix socket
so you can add events to it. The span is closed when the process exits from
timeout, (catchable) signals, or deliberate exit.

    socket_dir=$(mktemp -d)
	otel-cli span background \
		--service "my-long-script.sh" \
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
	// this used to be a global const but now it's in Config
	// TODO: does it make sense to make this configurable? 10ms might be too frequent...
	config.BackgroundParentPollMs = 10

	spanCmd.AddCommand(spanBgCmd)
	spanBgCmd.Flags().SortFlags = false
	// it seems like the socket should be required for background but it's
	// only necessary for adding events to the span. it should be fine to
	// start a background span at the top of a script then let it fall off
	// at the end to get an easy span
	spanBgCmd.Flags().StringVar(&config.BackgroundSockdir, "sockdir", defaults.BackgroundSockdir, "a directory where a socket can be placed safely")

	spanBgCmd.Flags().BoolVar(&config.BackgroundWait, "wait", defaults.BackgroundWait, "wait for background to be fully started and then return")
	spanBgCmd.Flags().BoolVar(&config.BackgroundSkipParentPidCheck, "skip-pid-check", defaults.BackgroundSkipParentPidCheck, "disable checking parent pid")

	addCommonParams(spanBgCmd)
	addSpanParams(spanBgCmd)
	addClientParams(spanBgCmd)
	addAttrParams(spanBgCmd)
}

// spanBgSockfile returns the full filename for the socket file under the
// provided background socket directory provided by the user.
func spanBgSockfile() string {
	return path.Join(config.BackgroundSockdir, spanBgSockfilename)
}

func doSpanBackground(cmd *cobra.Command, args []string) {
	// special case --wait, createBgClient() will wait for the socket to show up
	// then connect and send a no-op RPC. by this time e.g. --tp-carrier should
	// be all done and everything is ready to go without race conditions
	if config.BackgroundWait {
		client, shutdown := createBgClient()
		defer shutdown()
		err := client.Call("BgSpan.Wait", &struct{}{}, &struct{}{})
		if err != nil {
			softFail("error while waiting on span background: %s", err)
		}
		return
	}

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
	if !config.BackgroundSkipParentPidCheck {
		ppid := os.Getppid()
		go func() {
			for {
				time.Sleep(time.Duration(config.BackgroundParentPollMs) * time.Millisecond)

				// check if the parent process has changed, exit when it does
				cppid := os.Getppid()
				if cppid != ppid {
					spanBgEndEvent("parent_exited", span)
					bgs.Shutdown()
				}
			}
		}()
	}

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
	attrs := trace.WithAttributes([]attribute.KeyValue{
		{Key: attribute.Key("timeout.duration"), Value: attribute.StringValue(config.Timeout)},
	}...)
	span.AddEvent(name, attrs)
}
