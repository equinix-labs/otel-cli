package otelcli

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
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
	initialized bool
}

// Encode returns the traceparent as a W3C formatted string.
func (tp Traceparent) Encode() string {
	var sampling int
	if tp.Sampling {
		sampling = 1
	}
	traceId := tp.TraceIdString()
	spanId := tp.SpanIdString()
	return fmt.Sprintf("%02d-%s-%s-%02d", tp.Version, traceId, spanId, sampling)
}

// TraceIdString returns the trace id in string form.
func (tp Traceparent) TraceIdString() string {
	return hex.EncodeToString(tp.TraceId)
}

// SpanIdString returns the span id in string form.
func (tp Traceparent) SpanIdString() string {
	return hex.EncodeToString(tp.SpanId)
}

// traceparentFromSpan builds a Traceparent struct from the provided span.
func traceparentFromSpan(span *tracepb.Span) Traceparent {
	return Traceparent{
		Version:     0,
		TraceId:     span.TraceId,
		SpanId:      span.SpanId,
		Sampling:    config.IsRecording(), // TODO: is this the right thing?
		initialized: true,
	}
}

// loadTraceparent checks the environment first for TRACEPARENT then if filename
// isn't empty, it will read that file and look for a bare traceparent in that
// file.
func loadTraceparent(filename string) Traceparent {
	tp := loadTraceparentFromEnv()
	if filename != "" {
		fileTp := loadTraceparentFromFile(filename)
		if fileTp.initialized {
			tp = fileTp
		}
	}

	if config.TraceparentRequired {
		if tp.initialized {
			// return from here if everything looks ok, otherwise fall through to the log.Fatal
			if !bytes.Equal(tp.TraceId, emptyTraceId) && !bytes.Equal(tp.SpanId, emptySpanId) {
				return tp
			}
		}
		softFail("failed to find a valid traceparent carrier in either environment for file '%s' while it's required by --tp-required", filename)
	}
	return tp
}

// loadTraceparentFromFile reads a traceparent from filename and returns a
// context with the traceparent set. The format for the file as written is
// just a bare traceparent string. Whitespace, "export " and "TRACEPARENT=" are
// stripped automatically so the file can also be a valid shell snippet.
func loadTraceparentFromFile(filename string) Traceparent {
	file, err := os.Open(filename)
	if err != nil {
		// only fatal when the tp carrier file is required explicitly, otherwise
		// just silently return the unmodified context
		if config.TraceparentRequired {
			softFail("could not open file '%s' for read: %s", filename, err)
		} else {
			return Traceparent{}
		}
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
		return Traceparent{}
	}

	// clean 'export TRACEPARENT=' and 'TRACEPARENT=' off the output
	tp = strings.TrimPrefix(tp, "export ")
	tp = strings.TrimPrefix(tp, "TRACEPARENT=")

	if !traceparentRe.MatchString(tp) {
		softLog("file '%s' was read but does not contain a valid traceparent", filename)
		return Traceparent{}
	}

	return parseTraceparent(tp)
}

// saveToFile takes a context and filename and writes the tp from
// that context into the specified file.
func (tp Traceparent) saveToFile(filename string, span *tracepb.Span) {
	if filename == "" {
		return
	}

	file, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		softFail("failure opening file '%s' for write: %s", filename, err)
	}
	defer file.Close()

	printSpanData(file, tp, span)
}

// propagateTraceparent saves the traceparent to file if necessary, then prints
// span info to the console according to command-line args.
func propagateTraceparent(span *tracepb.Span, target io.Writer) {
	var tp Traceparent
	if config.IsRecording() {
		tp = traceparentFromSpan(span)
	} else {
		// when in non-recording mode, and there is a TP available, propagate that
		tp = loadTraceparent(config.TraceparentCarrierFile)
	}
	tp.saveToFile(config.TraceparentCarrierFile, span)

	if config.TraceparentPrint {
		printSpanData(target, tp, span)
	}
}

// printSpanData takes the provided strings and prints them in a consitent format,
// depending on which command line arguments were set.
func printSpanData(target io.Writer, tp Traceparent, span *tracepb.Span) {
	// --tp-export will print "export TRACEPARENT" so it's
	// one less step to print to a file & source, or eval
	var exported string
	if config.TraceparentPrintExport {
		exported = "export "
	}

	var traceId, spanId string
	if span != nil {
		// when in non-recording mode, the printed trace/span id should be all zeroes
		// and the input TP passes through
		// NOTE: this is preserved behavior from before protobuf spans, maybe this isn't
		// worth the trouble?
		traceId = hex.EncodeToString(span.TraceId)
		spanId = hex.EncodeToString(span.SpanId)
	} else {
		// in recording mode these will match the TP
		traceId = tp.TraceIdString()
		spanId = tp.SpanIdString()
	}
	fmt.Fprintf(target, "# trace id: %s\n#  span id: %s\n%sTRACEPARENT=%s\n", traceId, spanId, exported, tp.Encode())
}

// loadTraceparentFromEnv loads the traceparent from the environment variable
// TRACEPARENT and sets it in the returned Go context.
func loadTraceparentFromEnv() Traceparent {
	// don't load the envvar when --tp-ignore-env is set
	if config.TraceparentIgnoreEnv {
		return Traceparent{}
	}

	tp := os.Getenv("TRACEPARENT")
	if tp == "" {
		return Traceparent{}
	}

	return parseTraceparent(tp)
}

// parseTraceparent parses a string traceparent and returns the struct.
func parseTraceparent(tp string) Traceparent {
	var err error
	out := Traceparent{}

	parts := traceparentRe.FindStringSubmatch(tp)
	if len(parts) != 5 {
		softFail("could not parse invalid traceparent %q", tp)
	}

	out.Version, err = strconv.Atoi(parts[1])
	if err != nil {
		softFail("could not parse traceparent version component in %q", tp)
	}

	out.TraceId, err = hex.DecodeString(parts[2])
	if err != nil {
		softFail("could not parse traceparent trace id component in %q", tp)
	}

	out.SpanId, err = hex.DecodeString(parts[3])
	if err != nil {
		softFail("could not parse traceparent span id component in %q", tp)
	}

	sampleFlag, err := strconv.ParseInt(parts[4], 10, 64)
	if err != nil {
		softFail("could not parse traceparent sampling bits component in %q", tp)
	}
	out.Sampling = (sampleFlag == 1)

	// mark that this is a successfully parsed struct
	out.initialized = true

	return out
}
