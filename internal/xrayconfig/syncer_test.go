package xrayconfig

import (
	"context"
	"errors"
	"os"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

type mutableClientSource struct {
	clients []Client
}

func TestPeriodicSyncerFallsBackToRestartWhenDynamicAddFails(t *testing.T) {
	first := Client{ID: "client-1", Email: "first@example.com", Flow: DefaultFlow}
	second := Client{ID: "client-2", Email: "second@example.com", Flow: DefaultFlow}
	source := &mutableClientSource{clients: []Client{first}}
	adder := &recordingUserAdder{err: errors.New("handler service unavailable")}
	restarts := 0
	syncer, err := NewPeriodicSyncer(PeriodicOptions{
		Interval: time.Minute,
		Source:   source,
		Generator: Generator{
			Definition: DefaultDefinition(),
			OutputPath: t.TempDir() + "/config.json",
		},
		RestartCommand: []string{"restart-xray"},
		Runner: func(context.Context, []string) ([]byte, error) {
			restarts++
			return nil, nil
		},
		UserAdder: adder,
	})
	if err != nil {
		t.Fatalf("new syncer: %v", err)
	}
	if _, err := syncer.sync(context.Background()); err != nil {
		t.Fatalf("bootstrap sync: %v", err)
	}
	source.clients = []Client{first, second}
	if _, err := syncer.sync(context.Background()); err != nil {
		t.Fatalf("fallback sync: %v", err)
	}
	if restarts != 2 {
		t.Fatalf("expected bootstrap and fallback restarts, got %d", restarts)
	}
}

func (s *mutableClientSource) ListClients(context.Context) ([]Client, error) {
	return append([]Client(nil), s.clients...), nil
}

type recordingUserAdder struct {
	calls [][]Client
	err   error
}

func (a *recordingUserAdder) AddUsers(_ context.Context, _ Generator, clients []Client) error {
	a.calls = append(a.calls, append([]Client(nil), clients...))
	return a.err
}

func TestPeriodicSyncerUsesDynamicAPIForAdditionsAndRestartForRemovals(t *testing.T) {
	first := Client{ID: "client-1", Email: "first@example.com", Flow: DefaultFlow}
	second := Client{ID: "client-2", Email: "second@example.com", Flow: DefaultFlow}
	source := &mutableClientSource{clients: []Client{first}}
	adder := &recordingUserAdder{}
	restarts := 0
	syncer, err := NewPeriodicSyncer(PeriodicOptions{
		Interval: time.Minute,
		Source:   source,
		Generator: Generator{
			Definition: DefaultDefinition(),
			OutputPath: t.TempDir() + "/config.json",
		},
		RestartCommand: []string{"restart-xray"},
		Runner: func(context.Context, []string) ([]byte, error) {
			restarts++
			return nil, nil
		},
		UserAdder: adder,
	})
	if err != nil {
		t.Fatalf("new syncer: %v", err)
	}

	if _, err := syncer.sync(context.Background()); err != nil {
		t.Fatalf("bootstrap sync: %v", err)
	}
	if restarts != 1 {
		t.Fatalf("bootstrap must restart once, got %d", restarts)
	}

	source.clients = []Client{first, second}
	if _, err := syncer.sync(context.Background()); err != nil {
		t.Fatalf("addition sync: %v", err)
	}
	if restarts != 1 {
		t.Fatalf("pure addition restarted Xray; restarts=%d", restarts)
	}
	if len(adder.calls) != 1 || !reflect.DeepEqual(adder.calls[0], []Client{second}) {
		t.Fatalf("unexpected dynamic additions: %#v", adder.calls)
	}

	// Reordering the same set is persisted but does not alter runtime users.
	source.clients = []Client{second, first}
	if _, err := syncer.sync(context.Background()); err != nil {
		t.Fatalf("reorder sync: %v", err)
	}
	if restarts != 1 || len(adder.calls) != 1 {
		t.Fatalf("reorder changed runtime: restarts=%d additions=%d", restarts, len(adder.calls))
	}

	// Removal must restart so existing sessions belonging to the removed user
	// are terminated immediately, not merely denied on their next connection.
	source.clients = []Client{second}
	if _, err := syncer.sync(context.Background()); err != nil {
		t.Fatalf("removal sync: %v", err)
	}
	if restarts != 2 {
		t.Fatalf("removal must restart Xray, got %d restarts", restarts)
	}
}

func TestPeriodicSyncerTriggerRunsBeforeFallbackInterval(t *testing.T) {
	var calls atomic.Int32
	source := clientSourceFunc(func(context.Context) ([]Client, error) {
		calls.Add(1)
		return []Client{{ID: "client-1"}}, nil
	})
	syncer, err := NewPeriodicSyncer(PeriodicOptions{
		Interval: time.Hour,
		Source:   source,
		Generator: Generator{
			Definition: DefaultDefinition(),
			OutputPath: t.TempDir() + "/config.json",
		},
	})
	if err != nil {
		t.Fatalf("new syncer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, err := syncer.Start(ctx)
	if err != nil {
		t.Fatalf("start syncer: %v", err)
	}
	defer func() { _ = stop(context.Background()) }()

	deadline := time.Now().Add(time.Second)
	for calls.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	syncer.Trigger()
	for calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := calls.Load(); got < 2 {
		t.Fatalf("event trigger did not run reconciliation, calls=%d", got)
	}
}

type clientSourceFunc func(context.Context) ([]Client, error)

func (f clientSourceFunc) ListClients(ctx context.Context) ([]Client, error) {
	return f(ctx)
}

func TestCLIUserAdderValidatesXrayResult(t *testing.T) {
	adder, err := NewCLIUserAdder("xray", "127.0.0.1:10086")
	if err != nil {
		t.Fatalf("new CLI user adder: %v", err)
	}
	adder.runner = func(_ context.Context, command []string) ([]byte, error) {
		if len(command) != 5 || command[1] != "api" || command[2] != "adu" || command[3] != "--server=127.0.0.1:10086" {
			t.Fatalf("unexpected command: %#v", command)
		}
		payload, err := os.ReadFile(command[4])
		if err != nil {
			t.Fatalf("read dynamic payload: %v", err)
		}
		if len(payload) == 0 {
			t.Fatal("dynamic payload is empty")
		}
		return []byte("Added 1 user(s) in total.\n"), nil
	}

	err = adder.AddUsers(context.Background(), Generator{Definition: DefaultDefinition()}, []Client{{
		ID: "client-2", Email: "second@example.com", Flow: DefaultFlow,
	}})
	if err != nil {
		t.Fatalf("add users: %v", err)
	}
}
