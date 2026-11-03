// Package ingest takes source records in over the ingest contract, makes
// them durable in the journal and says so (ADR-0014). What the records mean
// is decided later, by the workers.
package ingest

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"lotbook.org/lotbook/internal/journal"
)

// contract is the version of the ingest contract a record is accepted under,
// kept with it in the journal.
const contract = "ingest/v1"

// maxBody bounds a request, compressed and decompressed alike.
const maxBody = 32 << 20

// sourceShape is the contract's pattern for a source.
var sourceShape = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)

// Journal takes records and returns once they are durable.
type Journal interface {
	Append(ctx context.Context, records []journal.Record) error
}

// Routes registers the ingest contract's endpoint, POST /v1/source-events,
// on mux.
//
// Of the contract's shape this checks the request's envelope and every
// record's source; the rest of each record is taken as sent.
func Routes(mux *http.ServeMux, j Journal) {
	mux.Handle("POST /v1/source-events", sourceEvents{j})
}

type sourceEvents struct{ journal Journal }

func (h sourceEvents) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body := io.Reader(http.MaxBytesReader(w, r.Body, maxBody))
	if r.Header.Get("Content-Encoding") == "gzip" {
		zr, err := gzip.NewReader(body)
		if err != nil {
			refuse(w, r, problem{Status: http.StatusBadRequest, Title: "The body is not gzip", Detail: err.Error()})
			return
		}
		defer func() {
			if err := zr.Close(); err != nil {
				trace.SpanFromContext(r.Context()).RecordError(err)
			}
		}()
		body = io.LimitReader(zr, maxBody)
	}

	records, p := parse(body)
	if p != nil {
		refuse(w, r, *p)
		return
	}

	traceParent := traceParentOf(r.Context())
	for i := range records {
		records[i].Contract = contract
		records[i].TraceParent = traceParent
	}
	if err := h.journal.Append(r.Context(), records); err != nil {
		trace.SpanFromContext(r.Context()).RecordError(err)
		w.Header().Set("Retry-After", "1")
		refuse(w, r, problem{
			Status: http.StatusServiceUnavailable,
			Title:  "The journal did not take the records",
			Detail: "Send the request again; a record that reaches the journal twice is harmless.",
		})
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// parse reads a request and returns its records, or the problem that refuses
// it.
func parse(body io.Reader) ([]journal.Record, *problem) {
	var req struct {
		Records []json.RawMessage `json:"records"`
	}
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return nil, &problem{Status: http.StatusBadRequest, Title: "The request is not a request of this contract", Detail: err.Error()}
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, &problem{Status: http.StatusBadRequest, Title: "The request is not a request of this contract", Detail: "something follows the request"}
	}
	if len(req.Records) == 0 {
		return nil, &problem{Status: http.StatusBadRequest, Title: "The request is not a request of this contract", Detail: "a request carries at least one record"}
	}

	records := make([]journal.Record, 0, len(req.Records))
	var wrong []recordError
	for i, raw := range req.Records {
		var rec struct {
			Source *string `json:"source"`
		}
		switch err := json.Unmarshal(raw, &rec); {
		case err != nil:
			wrong = append(wrong, recordError{Record: i, Pointer: "", Reason: "a record is a JSON object"})
		case rec.Source == nil:
			wrong = append(wrong, recordError{Record: i, Pointer: "/source", Reason: "a record names its source"})
		case !sourceShape.MatchString(*rec.Source):
			wrong = append(wrong, recordError{Record: i, Pointer: "/source", Reason: fmt.Sprintf("a source matches %s", sourceShape)})
		default:
			records = append(records, journal.Record{Source: *rec.Source, Body: raw})
		}
	}
	if len(wrong) > 0 {
		return nil, &problem{Status: http.StatusBadRequest, Title: "A record does not have the contract's shape", Errors: wrong}
	}
	return records, nil
}

// traceParentOf is the W3C traceparent of the span in ctx, the ingest
// request's (ADR-0008).
func traceParentOf(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	return carrier.Get("traceparent")
}

// problem is an RFC 9457 problem, as the contract answers it.
type problem struct {
	Title  string        `json:"title"`
	Status int           `json:"status"`
	Detail string        `json:"detail,omitempty"`
	Errors []recordError `json:"errors,omitempty"`
}

type recordError struct {
	Record  int    `json:"record"`
	Pointer string `json:"pointer,omitempty"`
	Reason  string `json:"reason"`
}

func refuse(w http.ResponseWriter, r *http.Request, p problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	if err := json.NewEncoder(w).Encode(p); err != nil {
		trace.SpanFromContext(r.Context()).RecordError(err)
	}
}
