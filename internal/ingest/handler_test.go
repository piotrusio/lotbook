package ingest_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"lotbook.org/lotbook/internal/ingest"
	"lotbook.org/lotbook/internal/journal"
	"lotbook.org/lotbook/internal/platform/telemetry"
)

const callerTrace = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

func TestSourceEvents_acceptsEveryExampleOfTheContract(t *testing.T) {
	for name, body := range contractExamples(t, "examples") {
		t.Run(name, func(t *testing.T) {
			j := &fakeJournal{}
			resp := post(t, j, body, nil)

			if resp.Code != http.StatusAccepted {
				t.Fatalf("status = %d; want 202; body: %s", resp.Code, resp.Body)
			}
			var sent struct{ Records []json.RawMessage }
			if err := json.Unmarshal(body, &sent); err != nil {
				t.Fatal(err)
			}
			if len(j.got) != len(sent.Records) {
				t.Fatalf("the journal got %d records; want %d", len(j.got), len(sent.Records))
			}
			for i, r := range j.got {
				var want struct{ Source string }
				if err := json.Unmarshal(sent.Records[i], &want); err != nil {
					t.Fatal(err)
				}
				if r.Source != want.Source {
					t.Errorf("record %d: source = %q; want %q", i, r.Source, want.Source)
				}
				if !jsonEqual(t, r.Body, sent.Records[i]) {
					t.Errorf("record %d: the journal got %s; want what was sent, %s", i, r.Body, sent.Records[i])
				}
				if r.Contract != "ingest/v1" {
					t.Errorf("record %d: contract = %q; want ingest/v1", i, r.Contract)
				}
				parts := strings.Split(r.TraceParent, "-")
				if len(parts) != 4 || parts[1] != "4bf92f3577b34da6a3ce929d0e0e4736" || parts[2] == "00f067aa0ba902b7" {
					t.Errorf("record %d: traceparent = %q; want the ingest span, in the caller's trace", i, r.TraceParent)
				}
			}
		})
	}
}

func TestSourceEvents_acceptsAGzippedRequest(t *testing.T) {
	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	if _, err := w.Write(contractExamples(t, "examples")["transaction"]); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	j := &fakeJournal{}

	resp := post(t, j, gz.Bytes(), map[string]string{"Content-Encoding": "gzip"})

	if resp.Code != http.StatusAccepted || len(j.got) == 0 {
		t.Fatalf("status = %d with %d records written; want 202 and the records; body: %s",
			resp.Code, len(j.got), resp.Body)
	}
}

func TestSourceEvents_refusesAMalformedRequestAndWritesNothing(t *testing.T) {
	type wrong struct {
		record  int
		pointer string
	}
	cases := map[string]struct {
		body string
		want *wrong
	}{
		"not JSON":              {body: `{"records": [`},
		"records not a list":    {body: `{"records": "many"}`},
		"a member not in v1":    {body: `{"records": [{"source": "plant-a.erp"}], "sender": "me"}`},
		"no source":             {body: `{"records": [{"source": "plant-a.erp"}, {"position": "1"}]}`, want: &wrong{1, "/source"}},
		"a source not in shape": {body: `{"records": [{"source": "Plant A"}]}`, want: &wrong{0, "/source"}},
	}
	for name, body := range contractExamples(t, "x-lotbook-refused-examples") {
		if name == "empty" { // the only refused example this skeleton checks
			cases["refused example "+name] = struct {
				body string
				want *wrong
			}{body: string(body)}
		}
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			j := &fakeJournal{}
			resp := post(t, j, []byte(c.body), nil)

			p := problem(t, resp, http.StatusBadRequest)
			if len(j.got) != 0 {
				t.Errorf("the journal got %d records from a refused request", len(j.got))
			}
			if c.want == nil {
				return
			}
			if len(p.Errors) == 0 || p.Errors[0].Record != c.want.record || p.Errors[0].Pointer != c.want.pointer {
				t.Errorf("errors = %+v; want record %d at %s", p.Errors, c.want.record, c.want.pointer)
			}
		})
	}
}

func TestSourceEvents_answers503WhenTheJournalDoesNotTakeTheRecords(t *testing.T) {
	j := &fakeJournal{err: errors.New("no broker")}

	resp := post(t, j, contractExamples(t, "examples")["event"], nil)

	problem(t, resp, http.StatusServiceUnavailable)
	if s, err := strconv.Atoi(resp.Header().Get("Retry-After")); err != nil || s < 0 {
		t.Errorf("Retry-After = %q; want a number of seconds", resp.Header().Get("Retry-After"))
	}
}

type fakeJournal struct {
	got []journal.Record
	err error
}

func (f *fakeJournal) Append(_ context.Context, records []journal.Record) error {
	if f.err != nil {
		return f.err
	}
	f.got = append(f.got, records...)
	return nil
}

func post(t *testing.T, j ingest.Journal, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	ingest.Routes(mux, j)
	h := telemetry.HTTP(sdktrace.NewTracerProvider(), mux)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/source-events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("traceparent", callerTrace)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)
	return resp
}

type problemBody struct {
	Title  string
	Status int
	Errors []struct {
		Record  int
		Pointer string
		Reason  string
	}
}

func problem(t *testing.T, resp *httptest.ResponseRecorder, status int) problemBody {
	t.Helper()
	if resp.Code != status {
		t.Fatalf("status = %d; want %d; body: %s", resp.Code, status, resp.Body)
	}
	if ct := resp.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q; want application/problem+json", ct)
	}
	var p problemBody
	if err := json.Unmarshal(resp.Body.Bytes(), &p); err != nil {
		t.Fatalf("the problem is not JSON: %v; body: %s", err, resp.Body)
	}
	if p.Status != status || p.Title == "" {
		t.Errorf("problem = %+v; want status %d and a title", p, status)
	}
	return p
}

// contractExamples reads the request examples of the ingest contract, under
// "examples" (accepted) or "x-lotbook-refused-examples" (refused).
func contractExamples(t *testing.T, kind string) map[string][]byte {
	t.Helper()
	raw, err := os.ReadFile("../../contracts/ingest/v1/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			RequestBody struct {
				Content map[string]map[string]json.RawMessage
			}
		}
		Components struct {
			Examples map[string]struct{ Value json.RawMessage }
		}
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var examples map[string]struct {
		Ref   string `json:"$ref"`
		Value json.RawMessage
	}
	media := doc.Paths["/v1/source-events"]["post"].RequestBody.Content["application/json"]
	if err := json.Unmarshal(media[kind], &examples); err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for name, ex := range examples {
		if ex.Ref != "" {
			ex.Value = doc.Components.Examples[strings.TrimPrefix(ex.Ref, "#/components/examples/")].Value
		}
		out[name] = ex.Value
	}
	if len(out) == 0 {
		t.Fatalf("the contract has no %s", kind)
	}
	return out
}

func jsonEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(x, y)
}
