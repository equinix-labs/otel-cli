package cmd

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"go.opentelemetry.io/otel"
)

func TestOtelCliCarrier(t *testing.T) {
	// make sure the environment variable isn't polluting test state
	os.Unsetenv("TRACEPARENT")

	carrier := NewOtelCliCarrier()
	carrier.Clear() // clean up after other tests

	// traceparent is the only key supported by OtelCliCarrier
	got := carrier.Get("traceparent")
	if got != "" {
		t.Errorf("got a non-empty traceparent value '%s' where empty string was expected", got)
	}

	// foobar isn't supported but should be accepted and ignored silently
	carrier.Set("foobar", "baz")
	// if it shows up with any value whatsoever, something has gone wrong
	if carrier.Get("foobar") != "" {
		t.Errorf("got a non-empty key value where empty string was expected")
	}

	// traceparent is supported so this should work fine
	tp := "00-b122b620341449410b9cd900c96d459d-aa21cda35388b694-01"
	carrier.Set("traceparent", tp)

	// even though 2 keys have been set at this point, the carrier only returns
	// one key, traceparent
	keys := carrier.Keys()
	if len(keys) != 1 || keys[0] != "traceparent" {
		t.Errorf("expected just one key from Keys() but instead got %q", keys)
	}

	// make sure the value round-trips in one piece
	got = carrier.Get("traceparent")
	if got != tp {
		t.Errorf("expected traceparent value '%s' but got '%s'", tp, got)
	}

	// it's impractical to test the internal state of otel-go, so the next best
	// thing is to round-trip our traceparent through it and make sure it comes
	// back as expected
	prop := otel.GetTextMapPropagator()
	ctx := prop.Extract(context.Background(), carrier)
	if ctx == nil {
		t.Errorf("expected a context but got nil, likely a problem in otel? this shouldn't happen...")
	}

	// try to round trip the traceparent back out of that context ^^
	rtCarrier := NewOtelCliCarrier()
	prop.Inject(ctx, rtCarrier)
	got = carrier.Get("traceparent")
	if got != tp {
		t.Errorf("round-tripping traceparent through a context failed, expected '%s', got '%s'", tp, got)
	}

	carrier.Clear() // clean up for other tests
}

func TestLoadTraceparent(t *testing.T) {
	// make sure the environment variable isn't polluting test state
	os.Unsetenv("TRACEPARENT")

	// trace id should not change, because there's no envvar and no file
	loadTraceparent(context.Background(), os.DevNull)
	if envTp != "" {
		t.Error("traceparent detected where there should be none")
	}

	// load from file only
	testFileTp := "00-f61fc53f926e07a9c3893b1a722e1b65-7a2d6a804f3de137-01"
	file, err := ioutil.TempFile(t.TempDir(), "go-test-otel-cli")
	if err != nil {
		t.Fatalf("unable to create tempfile for testing: %s", err)
	}
	defer os.Remove(file.Name())
	// write in the full shell snippet format so that stripping gets tested
	// in this pass too
	file.WriteString("export TRACEPARENT=" + testFileTp)
	file.Close()

	// actually do the test...
	fileCtx := loadTraceparent(context.Background(), file.Name())
	got := getTraceparent(fileCtx)
	if got != testFileTp {
		t.Errorf("loadTraceparent with file failed, expected '%s', got '%s'", testFileTp, got)
	}

	// load from environment only
	testEnvTp := "00-b122b620341449410b9cd900c96d459d-aa21cda35388b694-01"
	os.Setenv("TRACEPARENT", testEnvTp)
	envCtx := loadTraceparent(context.Background(), os.DevNull)
	got = getTraceparent(envCtx)
	if got != testEnvTp {
		t.Errorf("loadTraceparent with envvar failed, expected '%s', got '%s'", testEnvTp, got)
	}

	// now try with both file and envvar set by the previous tests
	// the file is expected to win
	bothCtx := loadTraceparent(context.Background(), file.Name())
	got = getTraceparent(bothCtx)
	if got != testFileTp {
		t.Errorf("loadTraceparent with file and envvar set to different values failed, expected '%s', got '%s'", testFileTp, got)
	}
}

func TestWriteTraceparentToFile(t *testing.T) {
	testTp := "00-ce1c6ae29edafc52eb6dd223da7d20b4-1c617f036253531c-01"

	// create a tempfile for messing with
	file, err := ioutil.TempFile(t.TempDir(), "go-test-otel-cli")
	if err != nil {
		t.Fatalf("unable to create tempfile for testing: %s", err)
	}
	file.Close()
	defer os.Remove(file.Name()) // not strictly necessary

	// set up a carrier and inject the traceparent
	prop := otel.GetTextMapPropagator()
	carrier := NewOtelCliCarrier()
	carrier.Set("traceparent", testTp)
	ctx := prop.Extract(context.Background(), carrier)

	saveTraceparentToFile(ctx, file.Name())

	// read the data back, it should just be the traceparent string
	data, err := ioutil.ReadFile(file.Name())
	if err != nil {
		t.Fatalf("failed to read tempfile '%s': %s", file.Name(), err)
	}
	if len(data) == 0 {
		t.Errorf("saveTraceparentToFile wrote %d bytes to the tempfile, expected %d", len(data), len(testTp))
	}
	if string(data) != testTp {
		t.Errorf("invalid data in traceparent file, expected '%s', got '%s'", testTp, data)
	}
}
