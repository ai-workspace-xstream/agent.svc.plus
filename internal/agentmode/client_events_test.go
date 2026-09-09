package agentmode

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestWatchUserConfigEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent-server/v1/users/events" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer agent-token" || r.Header.Get("X-Agent-ID") != "node-1" {
			t.Fatalf("missing agent authentication headers: %#v", r.Header)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: users-changed\ndata: rev-1\n\n: keepalive\n\ndata: rev-2\n\n")
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "agent-token", ClientOptions{
		Timeout: time.Second,
		AgentID: "node-1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	var revisions []string
	_ = client.WatchUserConfigEvents(context.Background(), func(revision string) {
		revisions = append(revisions, revision)
	})
	if !reflect.DeepEqual(revisions, []string{"rev-1", "rev-2"}) {
		t.Fatalf("unexpected revisions: %#v", revisions)
	}
}
