package otelcli

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/equinix-labs/otel-cli/otlpclient"
	"github.com/spf13/cobra"
)

// execCmd sets up the `otel-cli exec` command
func execCmd(config *otlpclient.Config) *cobra.Command {
	cmd := cobra.Command{
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

	addCommonParams(&cmd, config)
	addSpanParams(&cmd, config)
	addAttrParams(&cmd, config)
	addClientParams(&cmd, config)

	return &cmd
}

func doExec(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()
	config := getConfig(ctx)
	ctx, client := otlpclient.StartClient(ctx, config)

	// put the command in the attributes, before creating the span so it gets picked up
	config.Attributes["command"] = args[0]
	config.Attributes["arguments"] = ""

	var child *exec.Cmd
	if len(args) > 1 {
		// CSV-join the arguments to send as an attribute
		buf := bytes.NewBuffer([]byte{})
		csv.NewWriter(buf).WriteAll([][]string{args[1:]})
		config.Attributes["arguments"] = buf.String()

		child = exec.Command(args[0], args[1:]...)
	} else {
		child = exec.Command(args[0])
	}

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

	span := otlpclient.NewProtobufSpanWithConfig(config)

	// set the traceparent to the current span to be available to the child process
	if config.IsRecording() {
		tp := otlpclient.TraceparentFromProtobufSpan(config, span)
		child.Env = append(child.Env, fmt.Sprintf("TRACEPARENT=%s", tp.Encode()))
		// when not recording, and a traceparent is available, pass it through
	} else if !config.TraceparentIgnoreEnv {
		tp := otlpclient.LoadTraceparent(config, span)
		if tp.Initialized {
			child.Env = append(child.Env, fmt.Sprintf("TRACEPARENT=%s", tp.Encode()))
		}
	}

	if err := child.Run(); err != nil {
		config.SoftFail("command failed: %s", err)
	}
	span.EndTimeUnixNano = uint64(time.Now().UnixNano())

	err := otlpclient.SendSpan(ctx, client, config, span)
	if err != nil {
		config.SoftFail("unable to send span: %s", err)
	}

	// set the global exit code so main() can grab it and os.Exit() properly
	otlpclient.Diag.ExecExitCode = child.ProcessState.ExitCode()

	otlpclient.PropagateTraceparent(config, span, os.Stdout)
}
