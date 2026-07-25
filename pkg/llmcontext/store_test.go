package llmcontext

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "llm_context.db"))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("failed to close store: %v", err)
		}
	})
	return store
}

func TestUpsertSessionUsesStableRepoScopedID(t *testing.T) {
	store := newTestStore(t)
	identity := SessionIdentity{
		Provider:   "openai",
		WorkingDir: "/tmp/repo",
		GitRemote:  "git@github.com:afterdarksys/apiproxyd.git",
		GitBranch:  "main",
		GitCommit:  "abc123",
	}

	first, err := store.UpsertSession(identity)
	if err != nil {
		t.Fatalf("failed to upsert first session: %v", err)
	}

	identity.GitCommit = "def456"
	second, err := store.UpsertSession(identity)
	if err != nil {
		t.Fatalf("failed to upsert second session: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("expected commit changes to keep same session id, got %q and %q", first.ID, second.ID)
	}
	if second.GitCommit != "def456" {
		t.Fatalf("expected latest commit to be stored, got %q", second.GitCommit)
	}
	if second.RepoKey != strings.ToLower(identity.GitRemote) {
		t.Fatalf("expected repo key to prefer git remote, got %q", second.RepoKey)
	}
}

func TestBuildPacketPrioritizesRelevantStoredContext(t *testing.T) {
	store := newTestStore(t)
	session, err := store.UpsertSession(SessionIdentity{
		Provider:   "anthropic",
		WorkingDir: "/tmp/repo",
		GitBranch:  "main",
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	_, _ = store.AddEvent(AddEventInput{
		SessionID: session.ID,
		Kind:      "summary",
		Source:    "README.md",
		Content:   "The daemon caches network infrastructure API responses.",
	})
	_, _ = store.AddEvent(AddEventInput{
		SessionID: session.ID,
		Kind:      "decision",
		Source:    "design",
		Content:   "The llm context proxy stores session events by git repo and working directory.",
	})

	packet, err := store.BuildPacket(BuildPacketInput{
		SessionID: session.ID,
		Question:  "How should the LLM context proxy identify repo sessions?",
		MaxBytes:  600,
	})
	if err != nil {
		t.Fatalf("failed to build packet: %v", err)
	}

	if !strings.Contains(packet.Content, "Task Packet") {
		t.Fatalf("expected packet header, got %q", packet.Content)
	}
	if len(packet.Events) == 0 {
		t.Fatal("expected selected events")
	}
	if packet.Events[0].Source != "design" {
		t.Fatalf("expected relevant design event first, got %q", packet.Events[0].Source)
	}
}

func TestAddEventRejectsUnknownSession(t *testing.T) {
	store := newTestStore(t)

	_, err := store.AddEvent(AddEventInput{
		SessionID: "missing",
		Kind:      "summary",
		Content:   "this should not be stored",
	})
	if err == nil {
		t.Fatal("expected unknown session id to fail")
	}
}

func TestCachedResponseExpires(t *testing.T) {
	store := newTestStore(t)
	request := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)
	response := []byte(`{"choices":[]}`)

	if _, err := store.StoreCachedResponse("openai", "gpt-test", request, response, time.Millisecond, nil); err != nil {
		t.Fatalf("failed to store cached response: %v", err)
	}
	if _, err := store.GetCachedResponse("openai", "gpt-test", request); err != nil {
		t.Fatalf("expected cache hit before expiry: %v", err)
	}

	time.Sleep(5 * time.Millisecond)
	if _, err := store.GetCachedResponse("openai", "gpt-test", request); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected expired cache miss, got %v", err)
	}
}
