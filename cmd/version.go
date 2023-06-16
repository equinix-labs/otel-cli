package otelcmd

import "strings"

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
