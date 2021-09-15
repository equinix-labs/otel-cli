package cmd

import (
	"strings"
	"testing"
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

	if !strings.Contains(fsm["headers"], "123test=deadbeefcafe") {
		t.Errorf("expected attribute not found in flattened string map: %q", fsm)
		t.Fail()
	}
}
