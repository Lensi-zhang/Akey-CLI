package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreSeparatesCredentialTypes(t *testing.T) {
	store, _ := testStore(t)
	ordinary, err := store.Create(Profile{Name: "volcengine", Provider: "volcengine", CredentialType: CredentialAPI, BaseURL: "https://ark.cn-beijing.volces.com/api/v3", Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.Create(Profile{Name: "volcengine-agent-plan", Provider: "volcengine", CredentialType: CredentialAgentPlan, BaseURL: "https://ark.cn-beijing.volces.com/api/plan/v3", Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	if ordinary.ID == plan.ID {
		t.Fatal("ordinary and plan profiles must have distinct keyring identities")
	}
	loaded, err := store.Get(plan.Name)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CredentialType != CredentialAgentPlan {
		t.Fatalf("credential type was not preserved: %q", loaded.CredentialType)
	}
}

func TestStoreDefaultsAndValidatesCredentialType(t *testing.T) {
	store, _ := testStore(t)
	created, err := store.Create(Profile{Name: "legacy", Provider: "provider", BaseURL: "https://example.com", Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	if created.CredentialType != CredentialAPI {
		t.Fatalf("expected ordinary API credential by default, got %q", created.CredentialType)
	}
	if _, err := store.Create(Profile{Name: "invalid", Provider: "provider", CredentialType: "mixed", BaseURL: "https://example.com", Model: "model"}); err == nil {
		t.Fatal("expected invalid credential type to fail")
	}
}

func TestLoadLegacyProfileDefaultsCredentialType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	legacy := `{"version":1,"current_profile":"legacy","profiles":[{"id":"0123456789abcdef0123456789abcdef","name":"legacy","provider":"provider","base_url":"https://example.com","model":"model","env_prefix":"API","created_at":"2026-11-08T08:00:00Z","updated_at":"2026-11-08T08:00:00Z"}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	profile, err := NewStore(path).Get("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if profile.CredentialType != CredentialAPI {
		t.Fatalf("expected legacy profile to use ordinary API credentials, got %q", profile.CredentialType)
	}
}
