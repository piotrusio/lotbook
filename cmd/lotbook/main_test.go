package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

func TestRun_refusesAMissingOrUnknownRole(t *testing.T) {
	for _, args := range [][]string{nil, {"nonsense"}} {
		var stdout, stderr bytes.Buffer
		if code := run(t.Context(), args, &stdout, &stderr); code != 2 {
			t.Errorf("run(%q) = %d; want 2", args, code)
		}
		if !strings.Contains(stderr.String(), "usage: lotbook") {
			t.Errorf("run(%q) printed no usage; stderr: %s", args, stderr.String())
		}
	}
}

// The worker needs a PostgreSQL and a Kafka to start; it is run end to end
// in TestRun_aSourceEventReachesPostgreSQL.
func TestRun_eachRoleServesTheProbesAndStopsCleanly(t *testing.T) {
	for _, role := range []string{"ingest", "query"} {
		t.Run(role, func(t *testing.T) {
			addr, stop := start(t, role, "-kafka-brokers", "127.0.0.1:1")

			if got := status(t, http.MethodGet, "http://"+addr+"/readyz", ""); got != http.StatusOK {
				t.Errorf("GET /readyz = %d; want 200", got)
			}

			if code := stop(); code != 0 {
				t.Errorf("run exited %d; want 0", code)
			}
		})
	}
}

func TestRun_ingestServesTheIngestContract(t *testing.T) {
	addr, stop := start(t, "ingest", "-kafka-brokers", "127.0.0.1:1")
	defer stop()

	if got := status(t, http.MethodPost, "http://"+addr+"/v1/source-events", `{"records": [`); got != http.StatusBadRequest {
		t.Errorf("POST /v1/source-events with a malformed body = %d; want 400", got)
	}
}

func TestRun_refusesToStartWithoutWhatTheRoleNeeds(t *testing.T) {
	for _, c := range []struct {
		args    []string
		missing string
	}{
		{[]string{"ingest"}, "-kafka-brokers"},
		{[]string{"worker", "-kafka-brokers", "127.0.0.1:1"}, "-postgres-url"},
		{[]string{"worker", "-postgres-url", "postgres://127.0.0.1:1/lotbook"}, "-kafka-brokers"},
		{[]string{"migrate"}, "-postgres-url"},
	} {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		var stdout, stderr bytes.Buffer
		if code := run(ctx, append(c.args, "-addr", freeAddr(t)), &stdout, &stderr); code != 2 {
			t.Errorf("%v exited %d; want 2", c.args, code)
		}
		if !strings.Contains(stderr.String(), c.missing) {
			t.Errorf("%v: stderr does not say %s is missing: %s", c.args, c.missing, stderr.String())
		}
		cancel()
	}
}

func TestRun_aSourceEventReachesPostgreSQL(t *testing.T) {
	seeds, postgres := os.Getenv("LOTBOOK_TEST_KAFKA_BROKERS"), os.Getenv("LOTBOOK_TEST_POSTGRES_URL")
	if seeds == "" || postgres == "" {
		t.Skip("LOTBOOK_TEST_KAFKA_BROKERS and LOTBOOK_TEST_POSTGRES_URL are not set; this test needs a Kafka and a PostgreSQL")
	}
	topic := createTopic(t, seeds)
	postgres = freshSchema(t, postgres)
	kafka := []string{"-kafka-brokers", seeds, "-journal-topic", topic}

	var stdout, stderr bytes.Buffer
	if code := run(t.Context(), []string{"migrate", "-postgres-url", postgres}, &stdout, &stderr); code != 0 {
		t.Fatalf("migrate exited %d; stderr: %s; stdout: %s", code, stderr.String(), stdout.String())
	}
	ingest, stopIngest := start(t, "ingest", kafka...)
	defer stopIngest()
	_, stopWorker := start(t, "worker", append(kafka, "-postgres-url", postgres, "-worker-group", topic)...)

	if got := status(t, http.MethodPost, "http://"+ingest+"/v1/source-events", transactionExample(t)); got != http.StatusAccepted {
		t.Fatalf("POST /v1/source-events = %d; want 202", got)
	}

	conn, err := pgx.Connect(t.Context(), postgres)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(t.Context())
	var n int
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if err := conn.QueryRow(t.Context(), `SELECT count(*) FROM skeleton_records WHERE source = 'plant-a.erp'`).Scan(&n); err == nil && n == 3 {
			break
		}
	}
	if n != 3 {
		t.Errorf("PostgreSQL holds %d records of plant-a.erp; want the transaction's 3", n)
	}
	if code := stopWorker(); code != 0 {
		t.Errorf("the worker exited %d; want 0", code)
	}
}

// start runs a role on a free address until the returned stop is called,
// which returns its exit code.
func start(t *testing.T, role string, flags ...string) (string, func() int) {
	t.Helper()
	addr := freeAddr(t)
	ctx, cancel := context.WithCancel(t.Context())
	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	args := append([]string{role, "-addr", addr, "-drain", "0s", "-grace", "1s"}, flags...)
	go func() { done <- run(ctx, args, &stdout, &stderr) }()

	waitHealthy(t, "http://"+addr+"/healthz", done)
	return addr, func() int {
		cancel()
		select {
		case code := <-done:
			if code != 0 {
				t.Logf("stderr: %s; stdout: %s", stderr.String(), stdout.String())
			}
			return code
		case <-time.After(5 * time.Second):
			t.Fatal("run did not stop")
			return -1
		}
	}
}

func transactionExample(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../contracts/ingest/v1/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Components struct {
			Examples map[string]struct{ Value json.RawMessage }
		}
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return string(doc.Components.Examples["transaction"].Value)
}

func createTopic(t *testing.T, seeds string) string {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(strings.Split(seeds, ",")...))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	name := "lotbook-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	topic := kmsg.NewCreateTopicsRequestTopic()
	topic.Topic, topic.NumPartitions, topic.ReplicationFactor = name, 3, 1
	req := kmsg.NewPtrCreateTopicsRequest()
	req.Topics = append(req.Topics, topic)
	resp, err := req.RequestWith(t.Context(), cl)
	if err != nil {
		t.Fatal(err)
	}
	if err := kerr.ErrorForCode(resp.Topics[0].ErrorCode); err != nil {
		t.Fatalf("create topic %s: %v", name, err)
	}
	return name
}

// freshSchema makes a schema of its own in the database at postgres and
// returns a URL that works inside it.
func freshSchema(t *testing.T, postgres string) string {
	t.Helper()
	name := "test_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	admin, err := pgx.Connect(t.Context(), postgres)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(t.Context())
	if _, err := admin.Exec(t.Context(), "CREATE SCHEMA "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if c, err := pgx.Connect(ctx, postgres); err == nil {
			_, _ = c.Exec(ctx, "DROP SCHEMA "+name+" CASCADE")
			_ = c.Close(ctx)
		}
	})
	u, err := url.Parse(postgres)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", name)
	u.RawQuery = q.Encode()
	return u.String()
}

func freeAddr(t *testing.T) string {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func waitHealthy(t *testing.T, url string, done <-chan int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case code := <-done:
			t.Fatalf("run exited %d before serving", code)
		default:
		}
		if status(t, http.MethodGet, url, "") == http.StatusOK {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never answered 200", url)
}

func status(t *testing.T, method, url, body string) int {
	req, err := http.NewRequestWithContext(t.Context(), method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	return resp.StatusCode
}
