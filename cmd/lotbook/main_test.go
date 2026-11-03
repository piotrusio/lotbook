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
			addr := freeAddr(t)
			ctx, cancel := context.WithCancel(t.Context())
			var stdout, stderr bytes.Buffer
			done := make(chan int, 1)
			go func() {
				done <- run(ctx, []string{role, "-addr", addr, "-drain", "0s", "-grace", "1s"}, &stdout, &stderr)
			}()

			waitHealthy(t, "http://"+addr+"/healthz", done)
			if got := status(t, "http://"+addr+"/readyz"); got != http.StatusOK {
				t.Errorf("GET /readyz = %d; want 200", got)
			}

			cancel()
			select {
			case code := <-done:
				if code != 0 {
					t.Errorf("run exited %d; want 0; stderr: %s; stdout: %s", code, stderr.String(), stdout.String())
				}
			case <-time.After(5 * time.Second):
				t.Fatal("run did not stop")
			}
		})
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
		if status(t, url) == http.StatusOK {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never answered 200", url)
}

func status(t *testing.T, url string) int {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
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
