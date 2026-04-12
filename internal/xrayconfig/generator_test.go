package xrayconfig

import (
	"strings"
	"testing"
)

func TestGeneratorRenderRequiresClientEmail(t *testing.T) {
	generator := Generator{
		Definition: DefaultDefinition(),
		Domain:     "node-a.svc.plus",
	}

	_, err := generator.Render([]Client{{
		ID:   "550e8400-e29b-41d4-a716-446655440000",
		Flow: DefaultFlow,
	}})
	if err == nil {
		t.Fatal("expected render to fail when client email is missing")
	}
	if !strings.Contains(err.Error(), "email") {
		t.Fatalf("expected email validation error, got %v", err)
	}
}

func TestGeneratorRenderUsesEmailAsStatsKey(t *testing.T) {
	generator := Generator{
		Definition: DefaultDefinition(),
		Domain:     "node-a.svc.plus",
	}

	buf, err := generator.Render([]Client{{
		ID:    "550e8400-e29b-41d4-a716-446655440000",
		Email: "2cc7f0b2-69f5-4b02-beb5-df4dd62be7b1",
		Flow:  DefaultFlow,
	}})
	if err != nil {
		t.Fatalf("render config: %v", err)
	}

	rendered := string(buf)
	if !strings.Contains(rendered, `"email": "2cc7f0b2-69f5-4b02-beb5-df4dd62be7b1"`) {
		t.Fatalf("expected rendered config to include stats email key, got %s", rendered)
	}
}
