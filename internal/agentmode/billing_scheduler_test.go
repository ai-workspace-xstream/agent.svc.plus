package agentmode

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestBillingClientTriggersBothJobs(t *testing.T) {
	var mu sync.Mutex
	hits := map[string]int{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client, err := NewBillingClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("new billing client: %v", err)
	}

	if err := client.TriggerCollectAndRate(context.Background()); err != nil {
		t.Fatalf("trigger collect-and-rate: %v", err)
	}
	if err := client.TriggerReconcile(context.Background()); err != nil {
		t.Fatalf("trigger reconcile: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if hits["/v1/jobs/collect-and-rate"] != 1 {
		t.Fatalf("expected collect-and-rate hit, got %#v", hits)
	}
	if hits["/v1/jobs/reconcile"] != 1 {
		t.Fatalf("expected reconcile hit, got %#v", hits)
	}
}

func TestBillingSchedulerInvokesImmediateJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	hits := map[string]int{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewBillingClient(server.URL, time.Second)
	if err != nil {
		t.Fatalf("new billing client: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	startBillingSchedulers(ctx, client, billingScheduleConfig{
		httpTimeout:       time.Second,
		collectInterval:   50 * time.Millisecond,
		reconcileInterval: 50 * time.Millisecond,
	}, logger)

	time.Sleep(20 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if hits["/v1/jobs/collect-and-rate"] == 0 {
		t.Fatalf("expected collect-and-rate to be invoked, got %#v", hits)
	}
	if hits["/v1/jobs/reconcile"] == 0 {
		t.Fatalf("expected reconcile to be invoked, got %#v", hits)
	}
}
