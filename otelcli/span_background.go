package otelcli

import (
	"context"
	"os"
	"os/signal"
	"path"
	"strconv"
	"syscall"
	"time"

	"github.com/equinix-labs/otel-cli/otlpclient"
	"github.com/spf13/cobra"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// spanBgCmd represents the span background command
func spanBgCmd(config *otlpclient.Config) *cobra.Command {
	cmd := cobra.Command{
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

	defaults := otlpclient.DefaultConfig()
	cmd.Flags().SortFlags = false // don't sort subcommands

	// it seems like the socket should be required for background but it's
	// only necessary for adding events to the span. it should be fine to
	// start a background span at the top of a script then let it fall off
	// at the end to get an easy span
	cmd.Flags().StringVar(&config.BackgroundSockdir, "sockdir", defaults.BackgroundSockdir, "a directory where a socket can be placed safely")

	cmd.Flags().IntVar(&config.BackgroundParentPollMs, "parent-poll", defaults.BackgroundParentPollMs, "number of milliseconds to wait between checking for whether the parent process exited")
	cmd.Flags().BoolVar(&config.BackgroundWait, "wait", defaults.BackgroundWait, "wait for background to be fully started and then return")
	cmd.Flags().BoolVar(&config.BackgroundSkipParentPidCheck, "skip-pid-check", defaults.BackgroundSkipParentPidCheck, "disable checking parent pid")

	addCommonParams(&cmd, config)
	addSpanParams(&cmd, config)
	addClientParams(&cmd, config)
	addAttrParams(&cmd, config)

	return &cmd
}

func doSpanBackground(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()
	config := getConfig(ctx)
	started := time.Now()
	ctx, client := otlpclient.StartClient(ctx, config)

	// special case --wait, createBgClient() will wait for the socket to show up
	// then connect and send a no-op RPC. by this time e.g. --tp-carrier should
	// be all done and everything is ready to go without race conditions
	if config.BackgroundWait {
		client, shutdown := createBgClient(config)
		defer shutdown()
		err := client.Call("BgSpan.Wait", &struct{}{}, &struct{}{})
		if err != nil {
			config.SoftFail("error while waiting on span background: %s", err)
		}
		return
	}

	span := otlpclient.NewProtobufSpanWithConfig(config)

	// span background is a bit different from span/exec in that it might be
	// hanging out while other spans are created, so it does the traceparent
	// propagation before the server starts, instead of after
	otlpclient.PropagateTraceparent(config, span, os.Stdout)

	sockfile := path.Join(config.BackgroundSockdir, spanBgSockfilename)
	bgs := createBgServer(ctx, sockfile, span)

	// set up signal handlers to cleanly exit on SIGINT/SIGTERM etc
	signals := make(chan os.Signal, 1)
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
					rt := time.Since(started)
					spanBgEndEvent(ctx, span, "parent_exited", rt)
					bgs.Shutdown()
				}
			}
		}()
	}

	// start the timeout goroutine, this is a little late but the server
	// has to be up for this to make much sense
	if timeout := config.ParseCliTimeout(); timeout > 0 {
		go func() {
			time.Sleep(timeout)
			rt := time.Since(started)
			spanBgEndEvent(ctx, span, "timeout", rt)
			bgs.Shutdown()
		}()
	}

	// will block until bgs.Shutdown()
	bgs.Run()

	span.EndTimeUnixNano = uint64(time.Now().UnixNano())
	err := otlpclient.SendSpan(ctx, client, config, span)
	if err != nil {
		config.SoftFail("Sending span failed: %s", err)
	}
}

// spanBgEndEvent adds an event with the provided name, to the provided span
// with uptime.milliseconds and timeout.seconds attributes.
func spanBgEndEvent(ctx context.Context, span *tracepb.Span, name string, elapsed time.Duration) {
	config := getConfig(ctx)
	event := otlpclient.NewProtobufSpanEvent()
	event.Name = name
	event.Attributes = otlpclient.StringMapAttrsToProtobuf(map[string]string{
		"config.timeout":      config.Timeout,
		"otel-cli.runtime_ms": strconv.FormatInt(elapsed.Milliseconds(), 10),
	})

	span.Events = append(span.Events, event)
}
