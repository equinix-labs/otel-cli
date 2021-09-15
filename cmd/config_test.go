package cmd

import (
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

	if fsm["headers"] != "123test=deadbeefcafe" {
		t.Errorf("expected header value not found in flattened string map: %q", fsm)
		t.Fail()
	}
}
