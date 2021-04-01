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

otel-cli exec -n my-cool-thing -s interesting-step curl https://cool-service/api/v1/endpoint

otel-cli exec -s "outer span" 'otel-cli exec -s "inner span" sleep 1'

WARNING: this does not clean or validate the command at all before passing it
to sh -c and should not be passed any untrusted input`,
	Run:  doExec,
	Args: cobra.MinimumNArgs(1),
}

func init() {
	rootCmd.AddCommand(execCmd)

	// --name / -s, see span.go
	execCmd.Flags().StringVarP(&spanName, "name", "s", "todo-generate-default-span-names", "set the name of the span")
}

func doExec(cmd *cobra.Command, args []string) {
	ctx, shutdown := initTracer()
	defer shutdown()
	ctx = loadTraceparent(ctx, traceparentCarrierFile)
	tracer := otel.Tracer("otel-cli/exec")

	// joining the string here is kinda gross... but should be fine
	// there might be a better way in Cobra, maybe require passing it after a '--'?
	commandString := strings.Join(args, " ")

	kindOption := trace.WithSpanKind(otelSpanKind(spanKind))
	ctx, span := tracer.Start(ctx, spanName, kindOption)
	span.SetAttributes(cliAttrsToOtel(spanAttrs)...) // applies CLI attributes to the span

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
	if !traceparentIgnoreEnv {
		child.Env = append(child.Env, fmt.Sprintf("TRACEPARENT=%s", getTraceparent(ctx)))
	}

	if err := child.Run(); err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("command failed: %s", err))
		span.AddEvent("command failed")
	} else {
		span.SetStatus(codes.Ok, "success")
	}

	span.End()

	// set the global exit code so main() can grab it and os.Exit() properly
	exitCode = child.ProcessState.ExitCode()

	finishOtelCliSpan(ctx, span)
}
