package telemetry_test

import (
	"bytes"
	"encoding/json"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"lotbook.org/lotbook/internal/platform/telemetry"
)

func TestLogger_carriesTheTraceAndSpanOfTheContext(t *testing.T) {
	var buf bytes.Buffer
	log := telemetry.Logger(&buf).With("role", "ingest")
	tp := sdktrace.NewTracerProvider()
	ctx, span := tp.Tracer("test").Start(t.Context(), "work")

	log.InfoContext(ctx, "inside")
	span.End()
	log.InfoContext(t.Context(), "outside")

	lines := decode(t, &buf)
	sc := span.SpanContext()
	if lines[0]["trace_id"] != sc.TraceID().String() || lines[0]["span_id"] != sc.SpanID().String() {
		t.Errorf("inside a span the log carries trace_id %v, span_id %v; want %s, %s",
			lines[0]["trace_id"], lines[0]["span_id"], sc.TraceID(), sc.SpanID())
	}
	if lines[0]["role"] != "ingest" {
		t.Errorf("the log lost the attribute it was made with: %v", lines[0])
	}
	if _, ok := lines[1]["trace_id"]; ok {
		t.Errorf("outside a span the log carries a trace_id: %v", lines[1])
	}
}

func decode(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	dec := json.NewDecoder(buf)
	for dec.More() {
		var line map[string]any
		if err := dec.Decode(&line); err != nil {
			t.Fatalf("a log line is not JSON: %v", err)
		}
		lines = append(lines, line)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d log lines; want 2", len(lines))
	}
	return lines
}
