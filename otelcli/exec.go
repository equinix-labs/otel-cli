package otelcli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
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
	addCommonParams(execCmd)
	addSpanParams(execCmd)
	addAttrParams(execCmd)
	addClientParams(execCmd)
}

func doExec(cmd *cobra.Command, args []string) {
	// joining the string here is kinda gross... but should be fine
	// there might be a better way in Cobra, maybe require passing it after a '--'?
	commandString := strings.Join(args, " ")

	// put the command in the attributes, before creating the span so it gets picked up
	config.Attributes["command"] = commandString

	span := NewProtobufSpanWithConfig(config)

	// should this also work on Windows? for now, assume not
	child := exec.Command("/bin/sh", "-c", commandString)
	// attach all stdio to the parent's handles
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr

	// pass the existing env but add the latest TRACEPARENT carrier so e.g.
	// otel-cli exec 'otel-cli exec sleep 1' will relate the spans automatically
	child.Env = []string{}

	// grab everything BUT the TRACEPARENT envvar
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "TRACEPARENT=") {
			child.Env = append(child.Env, env)
		}
	}

	// set the traceparent to the current span to be available to the child process
	tp := traceparentFromSpan(span)
	child.Env = append(child.Env, fmt.Sprintf("TRACEPARENT=%s", tp.Encode()))

	if err := child.Run(); err != nil {
		span.Status.Code = tracepb.Status_STATUS_CODE_ERROR
		span.Status.Message = fmt.Sprintf("command failed: %s", err)
	}

	err := SendSpan(context.Background(), span)
	if err != nil {
		softFail("unable to send span: %s", err)
	}

	// set the global exit code so main() can grab it and os.Exit() properly
	diagnostics.ExecExitCode = child.ProcessState.ExitCode()

	propagateTraceparent(span, os.Stdout)
}
