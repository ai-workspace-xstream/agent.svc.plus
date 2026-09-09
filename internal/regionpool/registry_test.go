package regionpool

import (
	"testing"
	"time"
)

func testPools() []Pool {
	return []Pool{
		{Name: "jp", Region: "jpn-tky", Entrypoint: "JP-XConnect.svc.plus", Nodes: []Node{{ID: "jp-01", Enabled: true, Weight: 100}}},
		{Name: "us", Region: "us-ca", Entrypoint: "US-XConnect.svc.plus", Nodes: []Node{{ID: "us-01", Enabled: true, Weight: 100}}},
		{Name: "hk", Region: "hk", Entrypoint: "HK-XConnect.svc.plus", Nodes: []Node{{ID: "hk-01", Enabled: true, Weight: 100}}},
		{Name: "ph", Region: "ph-mnl", Entrypoint: "PH-XConnect.svc.plus", Nodes: []Node{{ID: "ph-surfercloud-01", Enabled: true, Weight: 100}}},
	}
}

func TestNormalizeRegionAndAliases(t *testing.T) {
	if got := NormalizeRegion("hp"); got != "ph-mnl" {
		t.Fatalf("NormalizeRegion(hp) = %q", got)
	}
	registry, err := NewRegistry(testPools(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := registry.Entrypoint("hp"); !ok || got != "PH-XConnect.svc.plus" {
		t.Fatalf("hp entrypoint = %q, %v", got, ok)
	}
}

func TestLifecycleAndHeartbeatSelection(t *testing.T) {
	now := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	registry, err := NewRegistry([]Pool{{Name: "ph", Region: "ph-mnl", Entrypoint: "PH-XConnect.svc.plus", Nodes: []Node{{ID: "a", Enabled: true}, {ID: "b", Enabled: true}}}}, nil, Options{Now: func() time.Time { return now }, HeartbeatTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Select("ph-mnl", "key"); err != ErrNoHealthyNode {
		t.Fatalf("select before heartbeat error = %v", err)
	}
	if err := registry.Heartbeat("a", Heartbeat{Healthy: true, Score: 90, ObservedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Select("ph", "key"); err != nil {
		t.Fatalf("select after heartbeat: %v", err)
	}
	if err := registry.Drain("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Select("ph", "key"); err != ErrNoHealthyNode {
		t.Fatalf("select while drained error = %v", err)
	}
	if err := registry.Undrain("a"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Disable("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Select("ph", "key"); err != ErrNoHealthyNode {
		t.Fatalf("select while disabled error = %v", err)
	}
	if err := registry.Enable("a"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Heartbeat("a", Heartbeat{Healthy: true, ObservedAt: now.Add(-2 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Select("ph", "key"); err != ErrNoHealthyNode {
		t.Fatalf("select with expired heartbeat error = %v", err)
	}
}

func TestStableWeightedSelection(t *testing.T) {
	now := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	registry, err := NewRegistry([]Pool{{Name: "ph", Region: "ph-mnl", Entrypoint: "PH-XConnect.svc.plus", Nodes: []Node{{ID: "a", Enabled: true, Weight: 1}, {ID: "b", Enabled: true, Weight: 3}}}}, nil, Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		if err := registry.Heartbeat(id, Heartbeat{Healthy: true, ObservedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := registry.Select("ph", "stable-key")
	if err != nil {
		t.Fatal(err)
	}
	second, err := registry.Select("ph", "stable-key")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("selection is not stable: %q then %q", first.ID, second.ID)
	}
}

func TestRegistryRejectsInvalidConfiguration(t *testing.T) {
	checks := []struct {
		name  string
		pools []Pool
	}{
		{"duplicate pool", []Pool{{Name: "ph", Region: "ph-mnl", Entrypoint: "PH"}, {Name: "ph", Region: "other", Entrypoint: "OTHER"}}},
		{"duplicate region", []Pool{{Name: "one", Region: "ph-mnl", Entrypoint: "PH"}, {Name: "two", Region: "hp", Entrypoint: "HP"}}},
		{"duplicate node", []Pool{{Name: "ph", Region: "ph-mnl", Entrypoint: "PH", Nodes: []Node{{ID: "same"}, {ID: "same"}}}}},
		{"negative weight", []Pool{{Name: "ph", Region: "ph-mnl", Entrypoint: "PH", Nodes: []Node{{ID: "bad", Weight: -1}}}}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if _, err := NewRegistry(check.pools, nil); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
