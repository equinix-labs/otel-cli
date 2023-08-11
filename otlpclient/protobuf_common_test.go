package otlpclient

import (
	"testing"
)

func TestCliAttrsToOtel(t *testing.T) {

	testAttrs := map[string]string{
		"test 1 - string":      "isn't testing fun?",
		"test 2 - int64":       "111111111",
		"test 3 - float":       "2.4391111",
		"test 4 - bool, true":  "true",
		"test 5 - bool, false": "false",
		"test 6 - bool, True":  "True",
		"test 7 - bool, False": "False",
	}

	otelAttrs := StringMapAttrsToProtobuf(testAttrs)

	// can't count on any ordering from map -> array
	for _, attr := range otelAttrs {
		key := string(attr.Key)
		switch key {
		case "test 1 - string":
			if attr.Value.GetStringValue() != testAttrs[key] {
				t.Errorf("expected value '%s' for key '%s' but got '%s'", testAttrs[key], key, attr.Value.GetStringValue())
			}
		case "test 2 - int64":
			if attr.Value.GetIntValue() != 111111111 {
				t.Errorf("expected value '%s' for key '%s' but got %d", testAttrs[key], key, attr.Value.GetIntValue())
			}
		case "test 3 - float":
			if attr.Value.GetDoubleValue() != 2.4391111 {
				t.Errorf("expected value '%s' for key '%s' but got %f", testAttrs[key], key, attr.Value.GetDoubleValue())
			}
		case "test 4 - bool, true":
			if attr.Value.GetBoolValue() != true {
				t.Errorf("expected value '%s' for key '%s' but got %t", testAttrs[key], key, attr.Value.GetBoolValue())
			}
		case "test 5 - bool, false":
			if attr.Value.GetBoolValue() != false {
				t.Errorf("expected value '%s' for key '%s' but got %t", testAttrs[key], key, attr.Value.GetBoolValue())
			}
		case "test 6 - bool, True":
			if attr.Value.GetBoolValue() != true {
				t.Errorf("expected value '%s' for key '%s' but got %t", testAttrs[key], key, attr.Value.GetBoolValue())
			}
		case "test 7 - bool, False":
			if attr.Value.GetBoolValue() != false {
				t.Errorf("expected value '%s' for key '%s' but got %t", testAttrs[key], key, attr.Value.GetBoolValue())
			}
		}
	}
}
