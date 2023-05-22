package otelcli

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

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
	// read the cached env from config, because os.Environ() has to be modified
	// to work around OTel libs reading envvars directly
	for _, env := range config.envBackup {
		if !strings.HasPrefix(env, "TRACEPARENT=") {
			child.Env = append(child.Env, env)
		}
	}

	span := NewProtobufSpanWithConfig(config)

	// set the traceparent to the current span to be available to the child process
	if config.IsRecording() {
		tp := traceparentFromSpan(span)
		child.Env = append(child.Env, fmt.Sprintf("TRACEPARENT=%s", tp.Encode()))
		// when not recording, and a traceparent is available, pass it through
	} else if !config.TraceparentIgnoreEnv {
		tp := loadTraceparent(config.TraceparentCarrierFile)
		if tp.initialized {
			child.Env = append(child.Env, fmt.Sprintf("TRACEPARENT=%s", tp.Encode()))
		}
	}

	if err := child.Run(); err != nil {
		span.Status.Code = tracepb.Status_STATUS_CODE_ERROR
		span.Status.Message = fmt.Sprintf("command failed: %s", err)
	}
	span.EndTimeUnixNano = uint64(time.Now().UnixNano())

	err := SendSpan(context.Background(), config, span)
	if err != nil {
		softFail("unable to send span: %s", err)
	}

	// set the global exit code so main() can grab it and os.Exit() properly
	diagnostics.ExecExitCode = child.ProcessState.ExitCode()

	propagateTraceparent(span, os.Stdout)
}
