package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// execCmd represents the span command
var execCmd = &cobra.Command{
	Use:   "exec",
	Short: "execute the command provided",
	Long: `execute the command provided after the subcommand inside a span, measuring
and reporting how long it took to run. The wrapping span's w3c traceparent is automatically
passed to the child process's environment as TRACEPARENT.

Examples:

otel-cli exec --name my-cool-thing --span interesting-step curl https://cool-service/api/v1/endpoint

otel-cli exec -s "outer span" 'otel-cli exec -s "inner span" sleep 1'

WARNING: this does not clean or validate the command at all before passing it
to sh -c and should not be passed any untrusted input`,
	Run:  doExec,
	Args: cobra.MinimumNArgs(1),
}

func init() {
	rootCmd.AddCommand(execCmd)

	// --span-name / -s
	addSpanNameParam(execCmd) // see span.go
}

func doExec(cmd *cobra.Command, args []string) {
	ctx, shutdown := initTracer()
	defer shutdown()
	ctx = loadTraceparentFromEnv(ctx)
	tracer := otel.Tracer("otel-cli/exec")

	// joining the string here is kinda gross... but should be fine
	// there might be a better way in Cobra, maybe require passing it after a '--'?
	commandString := strings.Join(args, " ")

	ctx, span := tracer.Start(ctx, spanName, trace.WithSpanKind(otelSpanKind()))
	span.SetAttributes(cliAttrsToOtel()...) // applies CLI attributes to the span

	// put the command in the attributes
	span.SetAttributes(attribute.KeyValue{
		Key:   attribute.Key("command"),
		Value: attribute.StringValue(commandString),
	})

	// should this also work on Windows? for now, assume not
	child := exec.Command("/bin/sh", "-c", commandString)
	// attach all stdio to the parent's handles
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr

	// pass the existing env but add the latest TRACEPARENT carrier so e.g.
	// otel-cli exec 'otel-cli exec sleep 1' will relate the spans automatically
	child.Env = os.Environ()
	if !ignoreTraceparentEnv {
		child.Env = append(child.Env, fmt.Sprintf("TRACEPARENT=%s", getTraceparent(ctx)))
	}

	if err := child.Run(); err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("command failed: %s", err))
		span.AddEvent("command failed")
	} else {
		span.SetStatus(codes.Ok, "success")
	}

	span.End()

	printSpanStdout(ctx, span)
	// TODO: figure out how to make sure this program exits with the same code as the child program
}
