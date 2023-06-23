// Package traceparent contains a lightweight implementation of W3C
// traceparent parsing, loading from files and environment, and the reverse.
package traceparent

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var traceparentRe *regexp.Regexp
var emptyTraceId = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
var emptySpanId = []byte{0, 0, 0, 0, 0, 0, 0, 0}

func init() {
	// only anchored at the front because traceparents can include more things
	// per the standard but only the first 4 are required for our uses
	traceparentRe = regexp.MustCompile("^([[:xdigit:]]{2})-([[:xdigit:]]{32})-([[:xdigit:]]{16})-([[:xdigit:]]{2})")
}

// Traceparent represents a parsed W3C traceparent.
type Traceparent struct {
	Version     int
	TraceId     []byte
	SpanId      []byte
	Sampling    bool
	Initialized bool
}

// Encode returns the traceparent as a W3C formatted string.
func (tp Traceparent) Encode() string {
	var sampling int
	var traceId, spanId string
	if tp.Sampling {
		sampling = 1
	}

	if len(tp.TraceId) == 0 {
		traceId = hex.EncodeToString(emptyTraceId)
	} else {
		traceId = tp.TraceIdString()
	}

	if len(tp.SpanId) == 0 {
		spanId = hex.EncodeToString(emptySpanId)
	} else {
		spanId = tp.SpanIdString()
	}

	return fmt.Sprintf("%02d-%s-%s-%02d", tp.Version, traceId, spanId, sampling)
}

// TraceIdString returns the trace id in string form.
func (tp Traceparent) TraceIdString() string {
	if len(tp.TraceId) == 0 {
		return hex.EncodeToString(emptyTraceId)
	} else {
		return hex.EncodeToString(tp.TraceId)
	}
}

// SpanIdString returns the span id in string form.
func (tp Traceparent) SpanIdString() string {
	if len(tp.SpanId) == 0 {
		return hex.EncodeToString(emptySpanId)
	} else {
		return hex.EncodeToString(tp.SpanId)
	}
}

// LoadFromFile reads a traceparent from filename and returns a
// context with the traceparent set. The format for the file as written is
// just a bare traceparent string. Whitespace, "export " and "TRACEPARENT=" are
// stripped automatically so the file can also be a valid shell snippet.
func LoadFromFile(filename string) (Traceparent, error) {
	file, err := os.Open(filename)
	if err != nil {
		errOut := fmt.Errorf("could not open file '%s' for read: %s", filename, err)
		// only fatal when the tp carrier file is required explicitly, otherwise
		// just silently return the unmodified context
		return Traceparent{}, errOut
	}
	defer file.Close()

	// only use the line that contains TRACEPARENT
	var tp string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// printSpanData emits comments with trace id and span id, ignore those
		if strings.HasPrefix(line, "#") {
			continue
		} else if strings.Contains(strings.ToUpper(line), "TRACEPARENT") {
			tp = line
			break
		}
	}

	// silently fail if no traceparent was found
	if tp == "" {
		return Traceparent{}, nil
	}

	// clean 'export TRACEPARENT=' and 'TRACEPARENT=' off the output
	tp = strings.TrimPrefix(tp, "export ")
	tp = strings.TrimPrefix(tp, "TRACEPARENT=")

	if !traceparentRe.MatchString(tp) {
		return Traceparent{}, fmt.Errorf("file '%s' was read but does not contain a valid traceparent", filename)
	}

	return Parse(tp)
}

// SaveToFile takes a context and filename and writes the tp from
// that context into the specified file.
func (tp Traceparent) SaveToFile(carrierFile string, export bool) error {
	file, err := os.OpenFile(carrierFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failure opening file '%s' for write: %w", carrierFile, err)
	}
	defer file.Close()

	return tp.Fprint(file, export)
}

// Fprint formats a traceparent into otel-cli's shell-compatible text format.
// If the second/export param is true, the statement will be prepended with "export "
// so it can be easily sourced in a shell script.
func (tp Traceparent) Fprint(target io.Writer, export bool) error {
	// --tp-export will print "export TRACEPARENT" so it's
	// one less step to print to a file & source, or eval
	var exported string
	if export {
		exported = "export "
	}

	traceId := tp.TraceIdString()
	spanId := tp.SpanIdString()
	_, err := fmt.Fprintf(target, "# trace id: %s\n#  span id: %s\n%sTRACEPARENT=%s\n", traceId, spanId, exported, tp.Encode())
	return err
}

// LoadFromEnv loads the traceparent from the environment variable
// TRACEPARENT and sets it in the returned Go context.
func LoadFromEnv() (Traceparent, error) {
	tp := os.Getenv("TRACEPARENT")
	if tp == "" {
		return Traceparent{}, nil
	}

	return Parse(tp)
}

// Parse parses a string traceparent and returns the struct.
func Parse(tp string) (Traceparent, error) {
	var err error
	out := Traceparent{}

	parts := traceparentRe.FindStringSubmatch(tp)
	if len(parts) != 5 {
		return out, fmt.Errorf("could not parse invalid traceparent %q", tp)
	}

	out.Version, err = strconv.Atoi(parts[1])
	if err != nil {
		return out, fmt.Errorf("could not parse traceparent version component in %q", tp)
	}

	out.TraceId, err = hex.DecodeString(parts[2])
	if err != nil {
		return out, fmt.Errorf("could not parse traceparent trace id component in %q", tp)
	}

	out.SpanId, err = hex.DecodeString(parts[3])
	if err != nil {
		return out, fmt.Errorf("could not parse traceparent span id component in %q", tp)
	}

	sampleFlag, err := strconv.ParseInt(parts[4], 10, 64)
	if err != nil {
		return out, fmt.Errorf("could not parse traceparent sampling bits component in %q", tp)
	}
	out.Sampling = (sampleFlag == 1)

	// mark that this is a successfully parsed struct
	out.Initialized = true

	return out, nil
}
