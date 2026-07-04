package agentmode

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"agent.svc.plus/internal/xrayconfig"
	"agent.svc.plus/internal/xrayembed"
)

type staticSource struct {
	clients []xrayconfig.Client
	calls   int
}

func (s *staticSource) ListClients(context.Context) ([]xrayconfig.Client, error) {
	s.calls++
	return s.clients, nil
}

// TestEmbeddedSyncStartsXray verifies the core monolith claim: a client set
// fetched from the (here, in-memory) source is rendered and hot-loaded into
// an in-process Xray instance that actually reaches the running state — no
// file write, no subprocess.
func TestEmbeddedSyncStartsXray(t *testing.T) {
	const tmpl = `{
		"log": {"loglevel": "warning"},
		"inbounds": [{
			"listen": "127.0.0.1",
			"port": 0,
			"protocol": "vless",
			"settings": {"clients": [], "decryption": "none"},
			"streamSettings": {"network": "tcp"}
		}],
		"outbounds": [{"protocol": "freedom"}]
	}`

	runner := xrayembed.New("test")
	t.Cleanup(func() { _ = runner.Close() })

	runners := map[string]*xrayembed.Runner{"test": runner}
	generators := map[string]xrayconfig.Generator{
		"test": {
			Definition: xrayconfig.JSONDefinition{Raw: []byte(tmpl)},
			OutputPath: "/dev/null",
		},
	}

	source := &staticSource{clients: []xrayconfig.Client{
		{ID: "b831381d-6324-4d53-ad4f-8cda48b30811", Email: "u@example.com"},
	}}

	ctx, cancel := context.WithCancel(context.Background())
	tracker := newSyncTracker()

	done := make(chan struct{})
	go func() {
		defer close(done)
		runXraySyncLoop(ctx, source, runners, generators, tracker, time.Hour, slog.Default())
	}()

	deadline := time.After(5 * time.Second)
	for {
		if runner.IsRunning() {
			break
		}
		select {
		case <-deadline:
			cancel()
			<-done
			t.Fatalf("embedded xray did not reach running state; source calls=%d, lastErr=%q",
				source.calls, tracker.Snapshot().LastError)
		case <-time.After(20 * time.Millisecond):
		}
	}

	if got := tracker.Snapshot().LastError; got != "" {
		t.Errorf("expected no sync error, got %q", got)
	}

	cancel()
	<-done
}
