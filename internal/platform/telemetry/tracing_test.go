package telemetry_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"lotbook.org/lotbook/internal/platform/telemetry"
)

type export struct{ method, path, contentType string }

func TestTracing_exportsOverOTLPHTTPWhenAnEndpointIsSet(t *testing.T) {
	got := make(chan export, 16)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		got <- export{r.Method, r.URL.Path, r.Header.Get("Content-Type")}
	}))
	defer collector.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)

	tp, err := telemetry.Tracing(t.Context(), "ingest")
	if err != nil {
		t.Fatal(err)
	}
	_, span := tp.Tracer("test").Start(t.Context(), "work")
	span.End()
	if err := tp.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	select {
	case e := <-got:
		want := export{http.MethodPost, "/v1/traces", "application/x-protobuf"}
		if e != want {
			t.Errorf("the collector got %+v; want %+v", e, want)
		}
	default:
		t.Fatal("nothing was exported to the collector")
	}
}

func TestTracing_withoutAnEndpointStillGivesSpansATrace(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")

	tp, err := telemetry.Tracing(t.Context(), "worker")
	if err != nil {
		t.Fatal(err)
	}
	_, span := tp.Tracer("test").Start(t.Context(), "work")
	span.End()
	if !span.SpanContext().IsValid() {
		t.Error("without an exporter the span has no trace, so logs could not be correlated")
	}
	if err := tp.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}
