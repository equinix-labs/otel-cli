package traceparent

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFprint(t *testing.T) {
	for _, tc := range []struct {
		tp     Traceparent
		export bool
		want   string
	}{
		// unconfigured, all zeroes
		{
			tp: Traceparent{
				Version:     0,
				TraceId:     []byte{},
				SpanId:      []byte{},
				Sampling:    false,
				Initialized: false,
			},
			export: false,
			want: "# trace id: 00000000000000000000000000000000\n" +
				"#  span id: 0000000000000000\n" +
				"TRACEPARENT=00-00000000000000000000000000000000-0000000000000000-00\n",
		},
		// fully loaded, print all the things
		{
			tp: Traceparent{
				Version:     0,
				TraceId:     []byte{0xfe, 0xdc, 0xcb, 0xa9, 0x87, 0x65, 0x43, 0x21, 0xfe, 0xdc, 0xcb, 0xa9, 0x87, 0x65, 0x43, 0x21},
				SpanId:      []byte{0xde, 0xea, 0xd6, 0xbb, 0xaa, 0xbb, 0xcc, 0xdd},
				Sampling:    true,
				Initialized: true,
			},
			export: true,
			want: "# trace id: fedccba987654321fedccba987654321\n" +
				"#  span id: deead6bbaabbccdd\n" +
				"export TRACEPARENT=00-fedccba987654321fedccba987654321-deead6bbaabbccdd-01\n",
		},
		// have a traceparent, but sampling is off, the tp should propagate as-is
		{
			tp: Traceparent{
				Version:     0,
				TraceId:     []byte{0xfe, 0xdc, 0xcb, 0xa9, 0x87, 0x65, 0x43, 0x21, 0xfe, 0xdc, 0xcb, 0xa9, 0x87, 0x65, 0x43, 0x21},
				SpanId:      []byte{0xde, 0xea, 0xd6, 0xbb, 0xaa, 0xbb, 0xcc, 0xdd},
				Sampling:    false,
				Initialized: true,
			},
			export: false,
			want: "# trace id: fedccba987654321fedccba987654321\n" +
				"#  span id: deead6bbaabbccdd\n" +
				// the traceparent provided should get printed
				"TRACEPARENT=00-fedccba987654321fedccba987654321-deead6bbaabbccdd-00\n",
		},
	} {
		buf := bytes.NewBuffer([]byte{})
		err := tc.tp.Fprint(buf, tc.export)
		if err != nil {
			t.Errorf("got an unexpected error: %s", err)
		}

		if diff := cmp.Diff(tc.want, buf.String()); diff != "" {
			t.Errorf("printed tp didn't match expected: (-want +got):\n%s", diff)
		}
	}
}

func TestLoadTraceparent(t *testing.T) {
	// make sure the environment variable isn't polluting test state
	os.Unsetenv("TRACEPARENT")

	// trace id should not change, because there's no envvar and no file
	tp, err := LoadFromFile(os.DevNull)
	if err != nil {
		t.Error("LoadFromFile returned an unexpected error: %w", err)
	}
	if tp.Initialized {
		t.Error("traceparent detected where there should be none")
	}

	// load from file only
	testFileTp := "00-f61fc53f926e07a9c3893b1a722e1b65-7a2d6a804f3de137-01"
	file, err := os.CreateTemp(t.TempDir(), "go-test-otel-cli")
	if err != nil {
		t.Fatalf("unable to create tempfile for testing: %s", err)
	}
	defer os.Remove(file.Name())
	// write in the full shell snippet format so that stripping gets tested
	// in this pass too
	file.WriteString("export TRACEPARENT=" + testFileTp)
	file.Close()

	// actually do the test...
	tp, err = LoadFromFile(file.Name())
	if err != nil {
		t.Error("LoadFromFile returned an unexpected error: %w", err)
	}
	if tp.Encode() != testFileTp {
		t.Errorf("LoadFromFile failed, expected '%s', got '%s'", testFileTp, tp.Encode())
	}

	// load from environment
	testEnvTp := "00-b122b620341449410b9cd900c96d459d-aa21cda35388b694-01"
	os.Setenv("TRACEPARENT", testEnvTp)
	tp, err = LoadFromEnv()
	if err != nil {
		t.Error("LoadFromEnv() returned an unexpected error: %w", err)
	}
	if tp.Encode() != testEnvTp {
		t.Errorf("LoadFromEnv() with envvar failed, expected '%s', got '%s'", testEnvTp, tp.Encode())
	}
}

func TestWriteTraceparentToFile(t *testing.T) {
	testTp := "00-ce1c6ae29edafc52eb6dd223da7d20b4-1c617f036253531c-01"
	tp, err := Parse(testTp)
	if err != nil {
		t.Errorf("failed while parsing test TP %q: %s", testTp, err)
	}

	// create a tempfile for messing with
	file, err := os.CreateTemp(t.TempDir(), "go-test-otel-cli")
	if err != nil {
		t.Fatalf("unable to create tempfile for testing: %s", err)
	}
	file.Close()
	defer os.Remove(file.Name()) // not strictly necessary

	err = tp.SaveToFile(file.Name(), false)
	if err != nil {
		t.Error("SaveToFile returned an unexpected error: %w", err)
	}

	// read the data back, it should just be the traceparent string
	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatalf("failed to read tempfile '%s': %s", file.Name(), err)
	}
	if len(data) == 0 {
		t.Errorf("saveTraceparentToFile wrote %d bytes to the tempfile, expected %d", len(data), len(testTp))
	}

	// otel is non-recording in tests so the comments in the output will be zeroed
	// while the traceparent should come through just fine at the end of file
	if !strings.HasSuffix(strings.TrimSpace(string(data)), testTp) {
		t.Errorf("invalid data in traceparent file, expected '%s', got '%s'", testTp, data)
	}
}
