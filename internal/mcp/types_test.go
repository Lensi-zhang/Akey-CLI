package mcp

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestBuiltinEntryUsesAkeyRun(t *testing.T) {
	server, err := Builtin("web-search", "volc-prod")
	if err != nil {
		t.Fatalf("Builtin() error = %v", err)
	}
	entry, err := server.Entry()
	if err != nil {
		t.Fatalf("Entry() error = %v", err)
	}
	wantArgs := []string{"run", "--profile", "volc-prod", "--", "web-search-mcp"}
	if entry.Command != "akey" || !reflect.DeepEqual(entry.Args, wantArgs) {
		t.Fatalf("Entry() = %#v, want command akey and args %#v", entry, wantArgs)
	}
	if _, managed := ManagedProfile(entry); !managed {
		t.Fatal("generated entry was not recognized as managed")
	}
	roundTrip, err := ServerFromEntry(server.Name, entry)
	if err != nil {
		t.Fatalf("ServerFromEntry() error = %v", err)
	}
	if !reflect.DeepEqual(roundTrip, server) {
		t.Fatalf("round trip = %#v, want %#v", roundTrip, server)
	}
}

func TestTemplatesAreControlledPlaceholders(t *testing.T) {
	templates := Templates()
	if len(templates) != 3 {
		t.Fatalf("len(Templates()) = %d, want 3", len(templates))
	}
	wantNames := []string{"web-search", "code-execute", "memory"}
	for i, template := range templates {
		if template.Name != wantNames[i] || template.Command == "" {
			t.Fatalf("template %d = %#v", i, template)
		}
		encoded, err := json.Marshal(template)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"api_key", "apiKey", "token", "secret", "password"} {
			if strings.Contains(strings.ToLower(string(encoded)), strings.ToLower(forbidden)) {
				t.Fatalf("template contains sensitive field %q", forbidden)
			}
		}
	}
}

func TestCustomServerToDocument(t *testing.T) {
	server, err := Custom("custom-server", "custom-mcp", []string{"--mode", "safe value"}, "dev")
	if err != nil {
		t.Fatalf("Custom() error = %v", err)
	}
	encoded, err := JSON([]Server{server})
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	var document Document
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	entry := document.MCPServers["custom-server"]
	wantArgs := []string{"run", "--profile", "dev", "--", "custom-mcp", "--mode", "safe value"}
	if entry.Command != "akey" || !reflect.DeepEqual(entry.Args, wantArgs) {
		t.Fatalf("entry = %#v, want args %#v", entry, wantArgs)
	}
	if strings.Contains(string(encoded), "fake-secret") || strings.Contains(string(encoded), "base_url") {
		t.Fatalf("unexpected sensitive or profile metadata in %s", encoded)
	}
}

func TestServerValidation(t *testing.T) {
	tests := []struct {
		name   string
		server Server
	}{
		{name: "uppercase name", server: Server{Name: "Bad", Command: "cmd", Profile: "dev"}},
		{name: "empty name", server: Server{Command: "cmd", Profile: "dev"}},
		{name: "invalid profile", server: Server{Name: "ok", Command: "cmd", Profile: "Bad"}},
		{name: "blank command", server: Server{Name: "ok", Command: " ", Profile: "dev"}},
		{name: "NUL command", server: Server{Name: "ok", Command: "bad\x00cmd", Profile: "dev"}},
		{name: "NUL arg", server: Server{Name: "ok", Command: "cmd", Args: []string{"bad\x00arg"}, Profile: "dev"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.server.Validate(); !errors.Is(err, ErrInvalidServer) {
				t.Fatalf("Validate() error = %v, want ErrInvalidServer", err)
			}
		})
	}
	if _, err := Builtin("unknown", "dev"); !errors.Is(err, ErrInvalidServer) {
		t.Fatalf("Builtin(unknown) error = %v", err)
	}
}

func TestManagedProfileRejectsExternalAndMalformedEntries(t *testing.T) {
	entries := []Entry{
		{Command: "other", Args: []string{"run", "--profile", "dev", "--", "cmd"}},
		{Command: "akey", Args: []string{"run", "--profile", "dev", "--"}},
		{Command: "akey", Args: []string{"run", "--profile", "Bad", "--", "cmd"}},
		{Command: "akey", Args: []string{"run", "--profile", "dev", "cmd"}},
		{Command: "akey", Type: "http", Args: []string{"run", "--profile", "dev", "--", "cmd"}},
	}
	for _, entry := range entries {
		if _, managed := ManagedProfile(entry); managed {
			t.Fatalf("ManagedProfile(%#v) = true", entry)
		}
	}
}
