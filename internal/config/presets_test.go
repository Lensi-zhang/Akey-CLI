package config

import (
	"net/url"
	"testing"

	"github.com/Lensi-zhang/Akey-CLI/internal/validate"
)

func TestProviderPresetsSeparateCredentialTypes(t *testing.T) {
	presets := ProviderPresets()
	if len(presets) < 6 {
		t.Fatalf("expected several provider presets, got %d", len(presets))
	}

	byID := make(map[string]ProviderPreset, len(presets))
	for _, preset := range presets {
		if _, exists := byID[preset.ID]; exists {
			t.Fatalf("duplicate preset ID %q", preset.ID)
		}
		byID[preset.ID] = preset
		if err := validate.ProfileName(preset.ID); err != nil {
			t.Fatalf("invalid preset ID %q: %v", preset.ID, err)
		}
		if preset.Name == "" || preset.Provider == "" || !preset.CredentialType.Valid() {
			t.Fatalf("incomplete preset: %+v", preset)
		}
		if preset.ID == "custom" {
			if preset.BaseURL != "" {
				t.Fatalf("custom preset must not provide a base URL: %q", preset.BaseURL)
			}
			if preset.ConsoleURL != "" {
				t.Fatalf("custom preset must not provide a console URL: %q", preset.ConsoleURL)
			}
			continue
		}
		if err := validate.BaseURL(preset.BaseURL); err != nil {
			t.Fatalf("invalid base URL for %q: %v", preset.ID, err)
		}
		consoleURL, err := url.Parse(preset.ConsoleURL)
		if err != nil || consoleURL.Scheme != "https" || consoleURL.Host == "" {
			t.Fatalf("invalid HTTPS console URL for %q: %q, err = %v", preset.ID, preset.ConsoleURL, err)
		}
	}

	ordinary := byID["volcengine"]
	agentPlan := byID["volcengine-agent-plan"]
	codingPlan := byID["volcengine-coding-plan"]
	if ordinary.CredentialType != CredentialAPI || agentPlan.CredentialType != CredentialAgentPlan || codingPlan.CredentialType != CredentialCodingPlan {
		t.Fatalf("ordinary and plan credential types are not distinct: %+v %+v %+v", ordinary, agentPlan, codingPlan)
	}
	if ordinary.Provider != agentPlan.Provider || ordinary.Provider != codingPlan.Provider {
		t.Fatal("Volcengine presets should retain the same provider identity")
	}
	if ordinary.BaseURL == agentPlan.BaseURL || ordinary.BaseURL == codingPlan.BaseURL || agentPlan.BaseURL == codingPlan.BaseURL {
		t.Fatal("ordinary, Agent Plan, and Coding Plan endpoints must be distinct")
	}
}

func TestProviderPresetsReturnsCopy(t *testing.T) {
	presets := ProviderPresets()
	presets[0].BaseURL = "https://invalid.example.com"
	if ProviderPresets()[0].BaseURL == presets[0].BaseURL {
		t.Fatal("caller mutated the shared preset catalog")
	}
}
