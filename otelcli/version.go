package otelcli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// versionCmd prints the version and exits.
func versionCmd(_ *Config) *cobra.Command {
	cmd := cobra.Command{
		Use:   "version",
		Short: "print otel-cli's version, commit, release date to stdout",
		Run:   doVersion,
	}

	return &cmd
}

func doVersion(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()
	config := getConfig(ctx)
	fmt.Fprintln(os.Stdout, config.Version)
}

// FormatVersion pretty-prints the global version, commit, and date values into
// a string to enable the --version flag. Public to be called from main.
func FormatVersion(version, commit, date string) string {
	parts := []string{}

	if version != "" {
		parts = append(parts, version)
	}

	if commit != "" {
		parts = append(parts, commit)
	}

	if date != "" {
		parts = append(parts, date)
	}

	if len(parts) == 0 {
		parts = append(parts, "unknown")
	}

	return strings.Join(parts, " ")
}
