package otelcli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"

	"github.com/equinix-labs/otel-cli/otlpclient"
	"github.com/equinix-labs/otel-cli/w3c/traceparent"
	"github.com/spf13/cobra"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

// execCmd sets up the `otel-cli exec` command
func execCmd(config *Config) *cobra.Command {
	cmd := cobra.Command{
		Use:   "exec",
		Short: "execute the command provided",
		Long: `execute the command provided after the subcommand inside a span, measuring
and reporting how long it took to run. The wrapping span's w3c traceparent is automatically
passed to the child process's environment as TRACEPARENT.

Examples:

otel-cli exec -n my-cool-thing -s interesting-step curl https://cool-service/api/v1/endpoint

otel-cli exec -s "outer span" 'otel-cli exec -s "inner span" sleep 1'`,
		Run:  doExec,
		Args: cobra.MinimumNArgs(1),
	}

	addCommonParams(&cmd, config)
	addSpanParams(&cmd, config)
	addAttrParams(&cmd, config)
	addClientParams(&cmd, config)

	defaults := DefaultConfig()
	cmd.Flags().StringVar(
		&config.ExecCommandTimeout,
		"command-timeout",
		defaults.ExecCommandTimeout,
		"timeout for the child process, when 0 otel-cli will wait forever",
	)

	cmd.Flags().BoolVar(
		&config.ExecTpDisableInject,
		"tp-disable-inject",
		defaults.ExecTpDisableInject,
		"disable automatically replacing {{traceparent}} with a traceparent",
	)

	return &cmd
}

func doExec(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()
	config := getConfig(ctx)
	span := config.NewProtobufSpan()

	// https://opentelemetry.io/docs/specs/semconv/attributes-registry/process/
	span.Attributes = []*commonpb.KeyValue{
		{
			Key: "process.command",
			Value: &commonpb.AnyValue{
				Value: &commonpb.AnyValue_StringValue{StringValue: args[0]},
			},
		},
		{ // will be overwritten if there are arguments
			Key: "process.command_args",
			Value: &commonpb.AnyValue{
				Value: &commonpb.AnyValue_ArrayValue{
					ArrayValue: &commonpb.ArrayValue{
						Values: []*commonpb.AnyValue{},
					},
				},
			},
		},
	}

	// no deadline if there is no command timeout set
	cancelCtxDeadline := func() {}
	// fork the context for the command so its deadline doesn't impact the otlpclient ctx
	cmdCtx := ctx
	cmdTimeout := config.ParseExecCommandTimeout()
	if cmdTimeout > 0 {
		cmdCtx, cancelCtxDeadline = context.WithDeadline(ctx, time.Now().Add(cmdTimeout))
	}

	// pass the existing env but add the latest TRACEPARENT carrier so e.g.
	// otel-cli exec 'otel-cli exec sleep 1' will relate the spans automatically
	childEnv := []string{}

	// set the traceparent to the current span to be available to the child process
	var tp traceparent.Traceparent
	if config.GetIsRecording() {
		tp = otlpclient.TraceparentFromProtobufSpan(span, config.GetIsRecording())
		childEnv = append(childEnv, fmt.Sprintf("TRACEPARENT=%s", tp.Encode()))
		// when not recording, and a traceparent is available, pass it through
	} else if !config.TraceparentIgnoreEnv {
		tp := config.LoadTraceparent()
		if tp.Initialized {
			childEnv = append(childEnv, fmt.Sprintf("TRACEPARENT=%s", tp.Encode()))
		}
	}

	var child *exec.Cmd
	if len(args) > 1 {
		tpArgs := make([]string, len(args)-1)

		if config.ExecTpDisableInject {
			copy(tpArgs, args[1:])
		} else {
			// loop over the args replacing {{traceparent}} with the current tp
			for i, arg := range args[1:] {
				tpArgs[i] = strings.Replace(arg, "{{traceparent}}", tp.Encode(), -1)
			}
		}

		// convert args to an OpenTelemetry string list
		// https://opentelemetry.io/docs/specs/semconv/attributes-registry/process/
		avlist := make([]*commonpb.AnyValue, len(tpArgs)+1)
		avlist[0] = &commonpb.AnyValue{
			Value: &commonpb.AnyValue_StringValue{
				StringValue: args[0],
			},
		}
		for i, v := range tpArgs {
			sv := commonpb.AnyValue_StringValue{StringValue: v}
			av := commonpb.AnyValue{Value: &sv}
			avlist[i+1] = &av
		}
		span.Attributes[1] = &commonpb.KeyValue{
			Key: "process.command_args",
			Value: &commonpb.AnyValue{
				Value: &commonpb.AnyValue_ArrayValue{
					ArrayValue: &commonpb.ArrayValue{
						Values: avlist,
					},
				},
			},
		}

		child = exec.CommandContext(cmdCtx, args[0], tpArgs...)
	} else {
		child = exec.CommandContext(cmdCtx, args[0])
	}

	// attach all stdio to the parent's handles
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr

	// grab everything BUT the TRACEPARENT envvar
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "TRACEPARENT=") {
			childEnv = append(childEnv, env)
		}
	}
	child.Env = childEnv

	// ctrl-c (sigint) is forwarded to the child process
	signals := make(chan os.Signal, 10)
	signalsDone := make(chan struct{})
	signal.Notify(signals, os.Interrupt)
	go func() {
		sig := <-signals
		child.Process.Signal(sig)
		// this might not seem necessary but without it, otel-cli exits before sending the span
		close(signalsDone)
	}()

	span.StartTimeUnixNano = uint64(time.Now().UnixNano())
	if err := child.Run(); err != nil {
		span.Status = &tracev1.Status{
			Message: fmt.Sprintf("exec command failed: %s", err),
			Code:    tracev1.Status_STATUS_CODE_ERROR,
		}
	}
	span.EndTimeUnixNano = uint64(time.Now().UnixNano())

	cancelCtxDeadline()
	close(signals)
	<-signalsDone

	// set --timeout on just the OTLP egress, starting now instead of process start time
	ctx, cancelCtxDeadline = context.WithDeadline(ctx, time.Now().Add(config.GetTimeout()))
	defer cancelCtxDeadline()

	ctx, client := StartClient(ctx, config)
	ctx, err := otlpclient.SendSpan(ctx, client, config, span)
	if err != nil {
		config.SoftFail("unable to send span: %s", err)
	}

	_, err = client.Stop(ctx)
	if err != nil {
		config.SoftFail("client.Stop() failed: %s", err)
	}

	// set the global exit code so main() can grab it and os.Exit() properly
	Diag.ExecExitCode = child.ProcessState.ExitCode()

	config.PropagateTraceparent(span, os.Stdout)
}
