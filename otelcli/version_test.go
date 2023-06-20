package otelcli

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFormatVersion(t *testing.T) {
	emptyVals := FormatVersion("", "", "")
	if diff := cmp.Diff("unknown", emptyVals); diff != "" {
		t.Fatalf("FormatVersion() mismatch (-want +got):\n%s", diff)
	}

	versionOnly := FormatVersion("0.0000", "", "")
	if diff := cmp.Diff("0.0000", versionOnly); diff != "" {
		t.Fatalf("FormatVersion() mismatch (-want +got):\n%s", diff)
	}

	loaded := FormatVersion("0.0000", "e48e468116baa5bd864f4057fc9a0f0774641f1a", "Wed Oct 5 12:28:07 2022 -0400")
	if diff := cmp.Diff("0.0000 e48e468116baa5bd864f4057fc9a0f0774641f1a Wed Oct 5 12:28:07 2022 -0400", loaded); diff != "" {
		t.Fatalf("FormatVersion() mismatch (-want +got):\n%s", diff)
	}
}
