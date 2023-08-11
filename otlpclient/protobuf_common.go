package otlpclient

// Implements just enough sugar on the OTel Protocol Buffers span definition
// to support otel-cli and no more.
//
// otel-cli does a few things that are awkward via the opentelemetry-go APIs
// which are restricted for good reasons.

import (
	"sort"
	"strconv"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
)

// StringMapAttrsToProtobuf takes a map of string:string, such as that from --attrs
// and returns them in an []*commonpb.KeyValue
func StringMapAttrsToProtobuf(attributes map[string]string) []*commonpb.KeyValue {
	out := []*commonpb.KeyValue{}

	for k, v := range attributes {
		av := new(commonpb.AnyValue)

		// try to parse as numbers, and fall through to string
		if i, err := strconv.ParseInt(v, 0, 64); err == nil {
			av.Value = &commonpb.AnyValue_IntValue{IntValue: i}
		} else if f, err := strconv.ParseFloat(v, 64); err == nil {
			av.Value = &commonpb.AnyValue_DoubleValue{DoubleValue: f}
		} else if b, err := strconv.ParseBool(v); err == nil {
			av.Value = &commonpb.AnyValue_BoolValue{BoolValue: b}
		} else {
			av.Value = &commonpb.AnyValue_StringValue{StringValue: v}
		}

		akv := commonpb.KeyValue{
			Key:   k,
			Value: av,
		}

		out = append(out, &akv)
	}

	return out
}

// AttrValueToString coverts a commonpb.KeyValue attribute to a string.
// Only used by tests for now.
func AttrValueToString(attr *commonpb.KeyValue) string {
	v := attr.GetValue()
	v.GetIntValue()
	if _, ok := v.Value.(*commonpb.AnyValue_StringValue); ok {
		return v.GetStringValue()
	} else if _, ok := v.Value.(*commonpb.AnyValue_IntValue); ok {
		return strconv.FormatInt(v.GetIntValue(), 10)
	} else if _, ok := v.Value.(*commonpb.AnyValue_DoubleValue); ok {
		return strconv.FormatFloat(v.GetDoubleValue(), byte('f'), -1, 64)
	}

	return ""
}

// flattenStringMap takes a string map and returns it flattened into a string with
// keys sorted lexically so it should be mostly consistent enough for comparisons
// and printing. Output is k=v,k=v style like attributes input.
func flattenStringMap(mp map[string]string, emptyValue string) string {
	if len(mp) == 0 {
		return emptyValue
	}

	var out string
	keys := make([]string, len(mp)) // for sorting
	var i int
	for k := range mp {
		keys[i] = k
		i++
	}
	sort.Strings(keys)

	for i, k := range keys {
		out = out + k + "=" + mp[k]
		if i == len(keys)-1 {
			break
		}
		out = out + ","
	}

	return out
}
