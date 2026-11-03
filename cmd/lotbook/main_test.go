package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
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

func TestRun_eachRoleServesTheProbesAndStopsCleanly(t *testing.T) {
	for _, role := range []string{"ingest", "worker", "query"} {
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

func TestRun_ingestRefusesToStartWithoutKafka(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	if code := run(ctx, []string{"ingest", "-addr", freeAddr(t)}, &stdout, &stderr); code != 2 {
		t.Errorf("ingest without -kafka-brokers exited %d; want 2", code)
	}
	if !strings.Contains(stderr.String(), "-kafka-brokers") {
		t.Errorf("stderr does not say what is missing: %s", stderr.String())
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
