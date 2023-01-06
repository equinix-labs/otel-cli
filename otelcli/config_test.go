package otelcli

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestConfig_ToStringMap(t *testing.T) {
	c := Config{}
	c.Headers = map[string]string{
		"123test": "deadbeefcafe",
	}

	fsm := c.ToStringMap()

	if _, ok := fsm["headers"]; !ok {
		t.Errorf("missing key 'headers' in returned string map: %q", fsm)
		t.Fail()
	}

	if fsm["headers"] != "123test=deadbeefcafe" {
		t.Errorf("expected header value not found in flattened string map: %q", fsm)
		t.Fail()
	}
}

func TestWithEndpoint(t *testing.T) {
	if DefaultConfig().WithEndpoint("foobar").Endpoint != "foobar" {
		t.Fail()
	}
}
func TestWithTimeout(t *testing.T) {
	if DefaultConfig().WithTimeout("foobar").Timeout != "foobar" {
		t.Fail()
	}
}
func TestWithHeaders(t *testing.T) {
	attr := map[string]string{"foo": "bar"}
	c := DefaultConfig().WithHeaders(attr)
	if diff := cmp.Diff(attr, c.Headers); diff != "" {
		t.Errorf("Headers did not match (-want +got):\n%s", diff)
	}
}
func TestWithInsecure(t *testing.T) {
	if DefaultConfig().WithInsecure(true).Insecure != true {
		t.Fail()
	}
}
func TestWithBlocking(t *testing.T) {
	if DefaultConfig().WithBlocking(true).Blocking != true {
		t.Fail()
	}
}
func TestWithNoTlsVerify(t *testing.T) {
	if DefaultConfig().WithNoTlsVerify(true).NoTlsVerify != true {
		t.Fail()
	}
}
func TestWithServiceName(t *testing.T) {
	if DefaultConfig().WithServiceName("foobar").ServiceName != "foobar" {
		t.Fail()
	}
}
func TestWithSpanName(t *testing.T) {
	if DefaultConfig().WithSpanName("foobar").SpanName != "foobar" {
		t.Fail()
	}
}
func TestWithKind(t *testing.T) {
	if DefaultConfig().WithKind("producer").Kind != "producer" {
		t.Fail()
	}
}
func TestWithAttributes(t *testing.T) {
	attr := map[string]string{"foo": "bar"}
	c := DefaultConfig().WithAttributes(attr)
	if diff := cmp.Diff(attr, c.Attributes); diff != "" {
		t.Errorf("Attributes did not match (-want +got):\n%s", diff)
	}
}

func TestWithStatusCode(t *testing.T) {
	if diff := cmp.Diff(DefaultConfig().WithStatusCode("unset").StatusCode, "unset"); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(DefaultConfig().WithStatusCode("ok").StatusCode, "ok"); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(DefaultConfig().WithStatusCode("error").StatusCode, "error"); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
}

func TestWithStatusDescription(t *testing.T) {
	if diff := cmp.Diff(DefaultConfig().WithStatusDescription("Set SCE To AUX").StatusDescription, "Set SCE To AUX"); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
}

func TestWithTraceparentCarrierFile(t *testing.T) {
	if DefaultConfig().WithTraceparentCarrierFile("foobar").TraceparentCarrierFile != "foobar" {
		t.Fail()
	}
}
func TestWithTraceparentIgnoreEnv(t *testing.T) {
	if DefaultConfig().WithTraceparentIgnoreEnv(true).TraceparentIgnoreEnv != true {
		t.Fail()
	}
}
func TestWithTraceparentPrint(t *testing.T) {
	if DefaultConfig().WithTraceparentPrint(true).TraceparentPrint != true {
		t.Fail()
	}
}
func TestWithTraceparentPrintExport(t *testing.T) {
	if DefaultConfig().WithTraceparentPrintExport(true).TraceparentPrintExport != true {
		t.Fail()
	}
}
func TestWithTraceparentRequired(t *testing.T) {
	if DefaultConfig().WithTraceparentRequired(true).TraceparentRequired != true {
		t.Fail()
	}
}
func TestWithBackgroundParentPollMs(t *testing.T) {
	if DefaultConfig().WithBackgroundParentPollMs(1111).BackgroundParentPollMs != 1111 {
		t.Fail()
	}
}
func TestWithBackgroundSockdir(t *testing.T) {
	if DefaultConfig().WithBackgroundSockdir("foobar").BackgroundSockdir != "foobar" {
		t.Fail()
	}
}
func TestWithBackgroundWait(t *testing.T) {
	if DefaultConfig().WithBackgroundWait(true).BackgroundWait != true {
		t.Fail()
	}
}
func TestWithSpanStartTime(t *testing.T) {
	if DefaultConfig().WithSpanStartTime("foobar").SpanStartTime != "foobar" {
		t.Fail()
	}
}
func TestWithSpanEndTime(t *testing.T) {
	if DefaultConfig().WithSpanEndTime("foobar").SpanEndTime != "foobar" {
		t.Fail()
	}
}
func TestWithEventName(t *testing.T) {
	if DefaultConfig().WithEventName("foobar").EventName != "foobar" {
		t.Fail()
	}
}
func TestWithEventTime(t *testing.T) {
	if DefaultConfig().WithEventTime("foobar").EventTime != "foobar" {
		t.Fail()
	}
}
func TestWithCfgFile(t *testing.T) {
	if DefaultConfig().WithCfgFile("foobar").CfgFile != "foobar" {
		t.Fail()
	}
}
func TestWithVerbose(t *testing.T) {
	if DefaultConfig().WithVerbose(true).Verbose != true {
		t.Fail()
	}
}
