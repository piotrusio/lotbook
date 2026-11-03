package server_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"lotbook.org/lotbook/internal/platform/server"
)

func TestRun_servesTheProbesAndTheHandler(t *testing.T) {
	ln := listen(t)
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	ctx, cancel := context.WithCancel(t.Context())
	done := start(ctx, ln, server.Config{Handler: h, Grace: time.Second})

	base := "http://" + ln.Addr().String()
	for path, want := range map[string]int{
		"/healthz": http.StatusOK,
		"/readyz":  http.StatusOK,
		"/other":   http.StatusTeapot,
	} {
		if got, err := get(t, base+path); err != nil || got != want {
			t.Errorf("GET %s = %d, %v; want %d", path, got, err, want)
		}
	}

	cancel()
	if err := wait(t, done); err != nil {
		t.Fatalf("Run returned %v; want nil", err)
	}
}

func TestRun_reportsNotReadyWhileDrainingThenStops(t *testing.T) {
	ln := listen(t)
	ctx, cancel := context.WithCancel(t.Context())
	done := start(ctx, ln, server.Config{Drain: 500 * time.Millisecond, Grace: time.Second})
	base := "http://" + ln.Addr().String()

	cancel()
	if !eventually(func() bool {
		got, err := get(t, base+"/readyz")
		return err == nil && got == http.StatusServiceUnavailable
	}) {
		t.Error("GET /readyz never answered 503 while draining")
	}
	if got, err := get(t, base+"/healthz"); err != nil || got != http.StatusOK {
		t.Errorf("GET /healthz while draining = %d, %v; want 200", got, err)
	}

	if err := wait(t, done); err != nil {
		t.Fatalf("Run returned %v; want nil", err)
	}
	if _, err := get(t, base+"/healthz"); err == nil {
		t.Error("the server still answers after Run returned")
	}
}

func TestRun_finishesRequestsInFlight(t *testing.T) {
	ln := listen(t)
	entered, release := make(chan struct{}), make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusOK)
	})
	ctx, cancel := context.WithCancel(t.Context())
	done := start(ctx, ln, server.Config{Handler: h, Grace: 5 * time.Second})

	answered := make(chan int, 1)
	go func() {
		code, _ := get(t, "http://"+ln.Addr().String()+"/slow")
		answered <- code
	}()
	<-entered
	cancel()
	select {
	case err := <-done:
		t.Fatalf("Run returned %v with a request in flight", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)

	if got := <-answered; got != http.StatusOK {
		t.Errorf("the request in flight got %d; want 200", got)
	}
	if err := wait(t, done); err != nil {
		t.Fatalf("Run returned %v; want nil", err)
	}
}

func TestRun_failsWhenRequestsOutliveTheGrace(t *testing.T) {
	ln := listen(t)
	entered, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { close(release) })
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
	})
	ctx, cancel := context.WithCancel(t.Context())
	done := start(ctx, ln, server.Config{Handler: h, Grace: 50 * time.Millisecond})

	go func() { _, _ = get(t, "http://"+ln.Addr().String()+"/stuck") }()
	<-entered
	cancel()

	if err := wait(t, done); err == nil {
		t.Fatal("Run returned nil although a request outlived the grace")
	}
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func start(ctx context.Context, ln net.Listener, cfg server.Config) <-chan error {
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx, ln, cfg) }()
	return done
}

func wait(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
		return nil
	}
}

func eventually(cond func() bool) bool {
	for deadline := time.Now().Add(400 * time.Millisecond); time.Now().Before(deadline); {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func get(t *testing.T, url string) (int, error) {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}
