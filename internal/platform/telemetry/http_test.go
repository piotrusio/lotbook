package telemetry_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"lotbook.org/lotbook/internal/platform/telemetry"
)

func TestHTTP_continuesTheCallersTraceAndNamesTheSpanByRoute(t *testing.T) {
	var inside trace.SpanContext
	mux := http.NewServeMux()
	mux.HandleFunc("GET /things/{id}", func(w http.ResponseWriter, r *http.Request) {
		inside = trace.SpanContextFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/things/7", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")

	s := serveOne(t, mux, req)

	if s.Name() != "GET /things/{id}" {
		t.Errorf("span name = %q; want %q", s.Name(), "GET /things/{id}")
	}
	if s.SpanKind() != trace.SpanKindServer {
		t.Errorf("span kind = %v; want server", s.SpanKind())
	}
	p := s.Parent()
	if !p.IsRemote() || p.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" || p.SpanID().String() != "00f067aa0ba902b7" {
		t.Errorf("parent = %v %v remote=%v; want the caller's traceparent", p.TraceID(), p.SpanID(), p.IsRemote())
	}
	if inside != s.SpanContext() {
		t.Error("the handler does not see the request's span in its context")
	}
	want := map[attribute.Key]attribute.Value{
		"http.request.method":       attribute.StringValue("GET"),
		"http.route":                attribute.StringValue("/things/{id}"),
		"http.response.status_code": attribute.IntValue(http.StatusNoContent),
	}
	got := map[attribute.Key]attribute.Value{}
	for _, kv := range s.Attributes() {
		got[kv.Key] = kv.Value
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("attribute %s = %v; want %v", k, got[k].Emit(), v.Emit())
		}
	}
}

func TestHTTP_startsATraceWhenTheCallerSendsNone(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /things", func(http.ResponseWriter, *http.Request) {})

	s := serveOne(t, mux, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/things", nil))

	if s.Parent().IsValid() {
		t.Errorf("the span has a parent %v although the caller sent none", s.Parent())
	}
	if !s.SpanContext().IsValid() {
		t.Error("the span has no valid trace")
	}
}

func TestHTTP_marksServerErrorsAndNotClientErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /broken", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	broken := serveOne(t, mux, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/broken", nil))
	if broken.Status().Code != codes.Error {
		t.Errorf("a 503 leaves the span %v; want error", broken.Status().Code)
	}

	missing := serveOne(t, mux, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/missing", nil))
	if missing.Status().Code == codes.Error {
		t.Error("a 404 marks the server span as an error")
	}
	if missing.Name() != "GET" {
		t.Errorf("a request matching no route is named %q; want %q", missing.Name(), "GET")
	}
}

func serveOne(t *testing.T, h http.Handler, req *http.Request) sdktrace.ReadOnlySpan {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	telemetry.HTTP(tp, h).ServeHTTP(httptest.NewRecorder(), req)
	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d spans; want 1", len(spans))
	}
	return spans[0]
}
