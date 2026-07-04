package xrayembed

import (
	"fmt"
	"testing"
)

// A minimal but real VLESS inbound on an ephemeral loopback port. The tag is
// what enables the live user-reload path.
func vlessConfig(port int, tag string, clients string) []byte {
	return []byte(fmt.Sprintf(`{
		"log": {"loglevel": "warning"},
		"inbounds": [{
			%s
			"listen": "127.0.0.1",
			"port": %d,
			"protocol": "vless",
			"settings": {"clients": [%s], "decryption": "none"},
			"streamSettings": {"network": "tcp"}
		}],
		"outbounds": [{"protocol": "freedom"}]
	}`, tag, port, clients))
}

const (
	userA = `{"id":"b831381d-6324-4d53-ad4f-8cda48b30811","email":"a@x.com"}`
	userB = `{"id":"e6f8a4d2-1c3b-4f5a-9d7e-2b0c1a8e5f42","email":"b@x.com"}`
)

// TestLiveUserReloadKeepsInstance is the core assertion of the feature: adding
// a user changes only the client set, so the running *core.Instance must be
// reused (no listener rebind, no disconnection) — its pointer is unchanged.
func TestLiveUserReloadKeepsInstance(t *testing.T) {
	r := New("test")
	t.Cleanup(func() { _ = r.Close() })

	if err := r.Apply(vlessConfig(0, `"tag":"vless-in",`, userA)); err != nil {
		t.Fatalf("initial apply: %v", err)
	}
	first := r.instance
	if first == nil || !r.IsRunning() {
		t.Fatal("expected running instance after initial apply")
	}

	// Add a second user — client-set-only change.
	if err := r.Apply(vlessConfig(0, `"tag":"vless-in",`, userA+","+userB)); err != nil {
		t.Fatalf("add-user apply: %v", err)
	}
	if r.instance != first {
		t.Fatal("instance was rebuilt on a client-only change; expected live reload")
	}
	if len(r.prevClients) != 2 {
		t.Fatalf("expected 2 tracked clients, got %d", len(r.prevClients))
	}

	// Remove the first user — still live.
	if err := r.Apply(vlessConfig(0, `"tag":"vless-in",`, userB)); err != nil {
		t.Fatalf("remove-user apply: %v", err)
	}
	if r.instance != first {
		t.Fatal("instance was rebuilt on a user removal; expected live reload")
	}
	if _, gone := r.prevClients["a@x.com"]; gone {
		t.Fatal("a@x.com should have been removed from tracked clients")
	}
}

// TestStructuralChangeRebuilds verifies the fallback: changing the listen port
// is structural, so the instance must be rebuilt (new pointer).
func TestStructuralChangeRebuilds(t *testing.T) {
	r := New("test")
	t.Cleanup(func() { _ = r.Close() })

	if err := r.Apply(vlessConfig(0, `"tag":"vless-in",`, userA)); err != nil {
		t.Fatalf("initial apply: %v", err)
	}
	first := r.instance

	// A different (still ephemeral) listen setup — change network to force a
	// structural diff without needing a fixed free port.
	structural := []byte(`{
		"log": {"loglevel": "warning"},
		"inbounds": [{
			"tag": "vless-in",
			"listen": "127.0.0.1",
			"port": 0,
			"protocol": "vless",
			"settings": {"clients": [` + userA + `], "decryption": "none"},
			"streamSettings": {"network": "ws"}
		}],
		"outbounds": [{"protocol": "freedom"}]
	}`)
	if err := r.Apply(structural); err != nil {
		t.Fatalf("structural apply: %v", err)
	}
	if r.instance == first {
		t.Fatal("expected a rebuilt instance on a structural (transport) change")
	}
}

// TestUntaggedInboundForcesRebuild confirms backward compatibility: without a
// tag the live path is unavailable, so every apply rebuilds.
func TestUntaggedInboundForcesRebuild(t *testing.T) {
	r := New("test")
	t.Cleanup(func() { _ = r.Close() })

	if err := r.Apply(vlessConfig(0, ``, userA)); err != nil {
		t.Fatalf("initial apply: %v", err)
	}
	first := r.instance
	if r.inboundTag != "" {
		t.Fatalf("expected empty inbound tag, got %q", r.inboundTag)
	}
	if err := r.Apply(vlessConfig(0, ``, userA+","+userB)); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if r.instance == first {
		t.Fatal("expected rebuild when inbound is untagged")
	}
}

func TestStructuralHashStableAcrossClientChurn(t *testing.T) {
	a, _ := parseConfig(vlessConfig(1443, `"tag":"vless-in",`, userA))
	b, _ := parseConfig(vlessConfig(1443, `"tag":"vless-in",`, userA+","+userB))
	ha, err := structuralHash(a)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := structuralHash(b)
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Fatal("structural hash changed on a client-only diff")
	}

	c, _ := parseConfig(vlessConfig(2443, `"tag":"vless-in",`, userA))
	hc, _ := structuralHash(c)
	if ha == hc {
		t.Fatal("structural hash should change when the listen port changes")
	}
}
