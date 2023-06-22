package traceparent

import (
	"os"
	"strings"
	"testing"
)

func TestLoadTraceparent(t *testing.T) {
	// make sure the environment variable isn't polluting test state
	os.Unsetenv("TRACEPARENT")

	// trace id should not change, because there's no envvar and no file
	tp, err := LoadAll(os.DevNull, false, false)
	if err != nil {
		t.Error("LoadAll returned an unexpected error: %w", err)
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
	tp, err = LoadAll(file.Name(), false, false)
	if err != nil {
		t.Error("LoadAll returned an unexpected error: %w", err)
	}
	if tp.Encode() != testFileTp {
		t.Errorf("loadTraceparent with file failed, expected '%s', got '%s'", testFileTp, tp.Encode())
	}

	// load from environment only
	testEnvTp := "00-b122b620341449410b9cd900c96d459d-aa21cda35388b694-01"
	os.Setenv("TRACEPARENT", testEnvTp)
	tp, err = LoadAll(os.DevNull, false, false)
	if err != nil {
		t.Error("LoadAll returned an unexpected error: %w", err)
	}
	if tp.Encode() != testEnvTp {
		t.Errorf("loadTraceparent with envvar failed, expected '%s', got '%s'", testEnvTp, tp.Encode())
	}

	// now try with both file and envvar set by the previous tests
	// the file is expected to win
	tp, err = LoadAll(file.Name(), false, false)
	if err != nil {
		t.Error("LoadAll returned an unexpected error: %w", err)
	}
	if tp.Encode() != testFileTp {
		t.Errorf("loadTraceparent with file and envvar set to different values failed, expected '%s', got '%s'", testFileTp, tp.Encode())
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

/*
func TestPropagateOtelCliSpan(t *testing.T) {
	// TODO: should this noop the tracing backend?

	config := DefaultConfig().
		WithTraceparentCarrierFile("").
		WithTraceparentPrint(false).
		WithTraceparentPrintExport(false)

	tp := "00-3433d5ae39bdfee397f44be5146867b3-8a5518f1e5c54d0a-01"
	tid := "3433d5ae39bdfee397f44be5146867b3"
	sid := "8a5518f1e5c54d0a"
	os.Setenv("TRACEPARENT", tp)
	//tracer := otel.Tracer("testing/propagateOtelCliSpan")
	//ctx, span := tracer.Start(context.Background(), "testing propagateOtelCliSpan")
	span := NewProtobufSpan()
	span.TraceId, _ = hex.DecodeString(tid)
	span.SpanId, _ = hex.DecodeString(sid)

	buf := new(bytes.Buffer)
	// mostly smoke testing this, will validate printSpanData output
	// TODO: maybe validate the file write works, but that's tested elsewhere...
	PropagateTraceparent(config, span, buf)
	if buf.Len() != 0 {
		t.Errorf("nothing was supposed to be written but %d bytes were", buf.Len())
	}

	config.TraceparentPrint = true
	config.TraceparentPrintExport = true
	buf = new(bytes.Buffer)
	ptp, _ := ParseTraceparent(tp)
	PrintSpanData(buf, ptp, span, config.TraceparentPrintExport)
	if buf.Len() == 0 {
		t.Error("expected more than zero bytes but got none")
	}
	expected := fmt.Sprintf("# trace id: %s\n#  span id: %s\nexport TRACEPARENT=%s\n", tid, sid, tp)
	if buf.String() != expected {
		t.Errorf("got unexpected output, expected '%s', got '%s'", expected, buf.String())
	}
}

*/
