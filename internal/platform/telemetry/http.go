package telemetry

import (
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

const scope = "lotbook.org/lotbook"

// HTTP gives every request served by next a server span from tp. The span
// continues the caller's trace when the request carries a W3C traceparent
// (ADR-0008) and starts one otherwise; it is named by the route next matched,
// and marked as an error when the answer is a 5xx.
func HTTP(tp trace.TracerProvider, next http.Handler) http.Handler {
	tracer := tp.Tracer(scope)
	var prop propagation.TraceContext
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := prop.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := tracer.Start(ctx, r.Method,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPRequestMethodKey.String(r.Method),
				semconv.URLPath(r.URL.Path),
			),
		)
		defer span.End()

		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		r = r.WithContext(ctx)
		next.ServeHTTP(sw, r)

		if route := route(r.Pattern); route != "" {
			span.SetName(r.Method + " " + route)
			span.SetAttributes(semconv.HTTPRoute(route))
		}
		span.SetAttributes(semconv.HTTPResponseStatusCode(sw.status))
		if sw.status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(sw.status))
		}
	})
}

// route is the path of a ServeMux pattern, "[METHOD ][HOST]/PATH".
func route(pattern string) string {
	if i := strings.IndexByte(pattern, '/'); i >= 0 {
		return pattern[i:]
	}
	return ""
}

// statusWriter remembers the status a handler answered with.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wrote && code >= http.StatusOK {
		w.status, w.wrote = code, true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
