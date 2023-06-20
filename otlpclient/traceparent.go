package otlpclient

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
	Initialized bool
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

// TraceparentFromSpan builds a Traceparent struct from the provided span.
func TraceparentFromSpan(span *tracepb.Span) Traceparent {
	return Traceparent{
		Version:     0,
		TraceId:     span.TraceId,
		SpanId:      span.SpanId,
		Sampling:    true, // TODO: fix this: hax
		Initialized: true,
	}
}

// LoadTraceparent checks the environment first for TRACEPARENT then if filename
// isn't empty, it will read that file and look for a bare traceparent in that
// file.
func LoadTraceparent(config Config) Traceparent {
	tp := loadTraceparentFromEnv(config)
	if config.TraceparentCarrierFile != "" {
		fileTp, err := loadTraceparentFromFile(config.TraceparentCarrierFile, config.TraceparentRequired)
		config.SoftFailIfErr(err)
		if fileTp.Initialized {
			tp = fileTp
		}
	}

	if config.TraceparentRequired {
		if tp.Initialized {
			// return from here if everything looks ok, otherwise fall through to the log.Fatal
			if !bytes.Equal(tp.TraceId, emptyTraceId) && !bytes.Equal(tp.SpanId, emptySpanId) {
				return tp
			}
		}
		config.SoftFail("failed to find a valid traceparent carrier in either environment for file '%s' while it's required by --tp-required", config.TraceparentCarrierFile)
	}
	return tp
}

// loadTraceparentFromFile reads a traceparent from filename and returns a
// context with the traceparent set. The format for the file as written is
// just a bare traceparent string. Whitespace, "export " and "TRACEPARENT=" are
// stripped automatically so the file can also be a valid shell snippet.
func loadTraceparentFromFile(filename string, tpRequired bool) (Traceparent, error) {
	file, err := os.Open(filename)
	if err != nil {
		errOut := fmt.Errorf("could not open file '%s' for read: %s", filename, err)
		Diag.SetError(errOut)
		// only fatal when the tp carrier file is required explicitly, otherwise
		// just silently return the unmodified context
		if tpRequired {
			return Traceparent{}, errOut
		} else {
			return Traceparent{}, nil // mask the error
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
		return Traceparent{}, nil
	}

	// clean 'export TRACEPARENT=' and 'TRACEPARENT=' off the output
	tp = strings.TrimPrefix(tp, "export ")
	tp = strings.TrimPrefix(tp, "TRACEPARENT=")

	if !traceparentRe.MatchString(tp) {
		return Traceparent{}, fmt.Errorf("file '%s' was read but does not contain a valid traceparent", filename)
	}

	return ParseTraceparent(tp)
}

// saveToFile takes a context and filename and writes the tp from
// that context into the specified file.
func (tp Traceparent) saveToFile(config Config, span *tracepb.Span) {
	if config.TraceparentCarrierFile == "" {
		return
	}

	file, err := os.OpenFile(config.TraceparentCarrierFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		config.SoftFail("failure opening file '%s' for write: %s", config.TraceparentCarrierFile, err)
	}
	defer file.Close()

	PrintSpanData(file, tp, span, config.TraceparentPrintExport)
}

// PropagateTraceparent saves the traceparent to file if necessary, then prints
// span info to the console according to command-line args.
func PropagateTraceparent(config Config, span *tracepb.Span, target io.Writer) {
	var tp Traceparent
	if config.IsRecording() {
		tp = TraceparentFromSpan(span)
	} else {
		// when in non-recording mode, and there is a TP available, propagate that
		tp = LoadTraceparent(config)
	}
	tp.saveToFile(config, span)

	if config.TraceparentPrint {
		PrintSpanData(target, tp, span, config.TraceparentPrintExport)
	}
}

// PrintSpanData takes the provided strings and prints them in a consitent format,
// depending on which command line arguments were set.
func PrintSpanData(target io.Writer, tp Traceparent, span *tracepb.Span, export bool) {
	// --tp-export will print "export TRACEPARENT" so it's
	// one less step to print to a file & source, or eval
	var exported string
	if export {
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
func loadTraceparentFromEnv(config Config) Traceparent {
	// don't load the envvar when --tp-ignore-env is set
	if config.TraceparentIgnoreEnv {
		return Traceparent{}
	}

	tp := os.Getenv("TRACEPARENT")
	if tp == "" {
		return Traceparent{}
	}

	tps, err := ParseTraceparent(tp)
	config.SoftFailIfErr(err)
	return tps
}

// ParseTraceparent parses a string traceparent and returns the struct.
func ParseTraceparent(tp string) (Traceparent, error) {
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
