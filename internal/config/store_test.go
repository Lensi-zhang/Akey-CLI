package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Lensi-zhang/Akey-CLI/internal/apperr"
)

func testStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nested", "profiles.json")
	return NewStore(path), path
}

func testProfile(name string) Profile {
	return Profile{
		Name:      name,
		Provider:  "volcengine",
		BaseURL:   "https://ark.cn-beijing.volces.com/api/v3",
		Model:     "doubao-pro-32k",
		EnvPrefix: "API",
		Note:      "test metadata",
	}
}

func stringPointer(value string) *string {
	return &value
}

func requireErrorKind(t *testing.T, err error, kind apperr.Kind) {
	t.Helper()
	var applicationError *apperr.Error
	if !errors.As(err, &applicationError) {
		t.Fatalf("error %v is not an apperr.Error", err)
	}
	if applicationError.Kind != kind {
		t.Fatalf("error kind = %q, want %q", applicationError.Kind, kind)
	}
}

func TestLoadMissingReturnsEmptyVersionOneState(t *testing.T) {
	store, path := testStore(t)
	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.Version != CurrentVersion || state.CurrentProfile != "" || state.Profiles == nil || len(state.Profiles) != 0 {
		t.Fatalf("unexpected empty state: %#v", state)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load created a configuration file: %v", err)
	}
}

func TestCreateGetListAndJSONSchema(t *testing.T) {
	store, path := testStore(t)
	input := testProfile("volc-prod")
	input.ID = strings.Repeat("f", 32)
	input.EnvPrefix = ""
	input.CreatedAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	input.UpdatedAt = input.CreatedAt

	created, err := store.Create(input)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !validID(created.ID) || created.ID == input.ID {
		t.Fatalf("Create did not generate a fresh 16-byte lowercase hex ID: %q", created.ID)
	}
	if created.EnvPrefix != DefaultEnvPrefix {
		t.Fatalf("EnvPrefix = %q, want %q", created.EnvPrefix, DefaultEnvPrefix)
	}
	if !isUTC(created.CreatedAt) || !isUTC(created.UpdatedAt) || created.CreatedAt.IsZero() {
		t.Fatalf("timestamps are not initialized in UTC: %#v", created)
	}

	got, err := store.Get(created.Name)
	if err != nil || !reflect.DeepEqual(got, created) {
		t.Fatalf("Get = %#v, %v; want %#v", got, err, created)
	}
	profiles, err := store.List()
	if err != nil || len(profiles) != 1 || !reflect.DeepEqual(profiles[0], created) {
		t.Fatalf("List = %#v, %v", profiles, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lowerJSON := strings.ToLower(string(data))
	for _, forbidden := range []string{`"api_key"`, `"apikey"`, `"token"`, `"secret"`, `"password"`} {
		if strings.Contains(lowerJSON, forbidden) {
			t.Fatalf("configuration contains forbidden credential field %s: %s", forbidden, data)
		}
	}
	var document struct {
		Version        int               `json:"version"`
		CurrentProfile string            `json:"current_profile"`
		Profiles       []json.RawMessage `json:"profiles"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("configuration is not valid JSON: %v", err)
	}
	if document.Version != 1 || document.CurrentProfile != "" || len(document.Profiles) != 1 {
		t.Fatalf("unexpected state JSON: %s", data)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(document.Profiles[0], &fields); err != nil {
		t.Fatalf("profile is not a JSON object: %v", err)
	}
	wantFields := []string{"base_url", "created_at", "env_prefix", "id", "model", "name", "note", "provider", "updated_at"}
	gotFields := make([]string, 0, len(fields))
	for field := range fields {
		gotFields = append(gotFields, field)
	}
	sort.Strings(gotFields)
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Fatalf("profile JSON fields = %v, want %v", gotFields, wantFields)
	}

	if runtime.GOOS != "windows" {
		fileInfo, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat file: %v", err)
		}
		if got := fileInfo.Mode().Perm(); got != 0o600 {
			t.Fatalf("file mode = %04o, want 0600", got)
		}
		directoryInfo, err := os.Stat(filepath.Dir(path))
		if err != nil {
			t.Fatalf("Stat directory: %v", err)
		}
		if got := directoryInfo.Mode().Perm(); got != 0o700 {
			t.Fatalf("directory mode = %04o, want 0700", got)
		}
	}
	if _, err := os.Stat(path + ".lock"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock file was not released: %v", err)
	}
}

func TestCreateRejectsDuplicateWithoutChangingState(t *testing.T) {
	store, path := testStore(t)
	if _, err := store.Create(testProfile("duplicate")); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Create(testProfile("duplicate"))
	requireErrorKind(t, err, apperr.KindConflict)
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("duplicate Create changed the persisted state")
	}
}

func TestUpdateOnlyAppliesProvidedPatchFields(t *testing.T) {
	store, _ := testStore(t)
	created, err := store.Create(testProfile("patch-me"))
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.Update(created.Name, Patch{
		Provider: stringPointer("openai"),
		BaseURL:  stringPointer("https://api.openai.com/v1"),
		Note:     stringPointer(""),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.ID != created.ID || updated.Name != created.Name || !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Fatal("Update changed immutable identity or creation time")
	}
	if updated.Provider != "openai" || updated.BaseURL != "https://api.openai.com/v1" || updated.Note != "" {
		t.Fatalf("Update did not apply patch: %#v", updated)
	}
	if updated.Model != created.Model || updated.EnvPrefix != created.EnvPrefix {
		t.Fatal("Update changed fields absent from the patch")
	}

	if _, err := store.Update(created.Name, Patch{BaseURL: stringPointer("https://user:FAKE_TEST_KEY@example.com")}); err == nil {
		t.Fatal("Update accepted credentials in BaseURL")
	} else if strings.Contains(err.Error(), "FAKE_TEST_KEY") {
		t.Fatalf("Update error leaked URL credentials: %v", err)
	}
	persisted, err := store.Get(created.Name)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.BaseURL != updated.BaseURL {
		t.Fatal("failed Update changed the persisted BaseURL")
	}
}

func TestRenamePreservesIDAndCurrentProfile(t *testing.T) {
	store, path := testStore(t)
	created, err := store.Create(testProfile("old-name"))
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.Create(testProfile("other"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Use(created.Name); err != nil {
		t.Fatal(err)
	}
	renamed, err := store.Rename(created.Name, "new-name")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if renamed.ID != created.ID || !renamed.CreatedAt.Equal(created.CreatedAt) {
		t.Fatal("Rename changed profile identity")
	}
	if _, err := store.Get("old-name"); err == nil {
		t.Fatal("old name still resolves after Rename")
	} else {
		requireErrorKind(t, err, apperr.KindNotFound)
	}
	current, err := store.Current()
	if err != nil || current.Name != "new-name" || current.ID != created.ID {
		t.Fatalf("Current after Rename = %#v, %v", current, err)
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rename("new-name", other.Name); err == nil {
		t.Fatal("Rename allowed a duplicate name")
	} else {
		requireErrorKind(t, err, apperr.KindConflict)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("conflicting Rename changed the persisted state")
	}
}

func TestUseCurrentAndDelete(t *testing.T) {
	store, _ := testStore(t)
	if _, err := store.Current(); err == nil {
		t.Fatal("Current succeeded without a default profile")
	} else {
		requireErrorKind(t, err, apperr.KindNotFound)
	}
	first, err := store.Create(testProfile("first"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Create(testProfile("second"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Use(first.Name); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(second.Name); err != nil {
		t.Fatalf("Delete non-current profile: %v", err)
	}
	current, err := store.Current()
	if err != nil || current.Name != first.Name {
		t.Fatalf("deleting a non-current profile changed default: %#v, %v", current, err)
	}
	if err := store.Delete(first.Name); err != nil {
		t.Fatalf("Delete current profile: %v", err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.CurrentProfile != "" || len(state.Profiles) != 0 || state.Profiles == nil {
		t.Fatalf("deleting current profile did not clear default safely: %#v", state)
	}
	if _, err := store.Current(); err == nil {
		t.Fatal("Current silently selected another profile after deletion")
	}
}

func TestConcurrentCreateAndAtomicReads(t *testing.T) {
	store, path := testStore(t)
	const profileCount = 16

	stopReader := make(chan struct{})
	readerDone := make(chan struct{})
	readerErrors := make(chan error, 1)
	go func() {
		defer close(readerDone)
		for {
			select {
			case <-stopReader:
				return
			default:
				if _, err := store.Load(); err != nil {
					readerErrors <- err
					return
				}
			}
		}
	}()

	start := make(chan struct{})
	errorsByCreate := make(chan error, profileCount)
	var writers sync.WaitGroup
	for i := 0; i < profileCount; i++ {
		writers.Add(1)
		go func(index int) {
			defer writers.Done()
			<-start
			isolatedStore := NewStore(path)
			_, err := isolatedStore.Create(testProfile(fmt.Sprintf("profile-%02d", index)))
			errorsByCreate <- err
		}(i)
	}
	close(start)
	writers.Wait()
	close(stopReader)
	<-readerDone
	close(errorsByCreate)
	for err := range errorsByCreate {
		if err != nil {
			t.Fatalf("concurrent Create: %v (cause: %v)", err, errors.Unwrap(err))
		}
	}
	select {
	case err := <-readerErrors:
		t.Fatalf("concurrent Load observed invalid state: %v (cause: %v)", err, errors.Unwrap(err))
	default:
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("final Load: %v", err)
	}
	if len(state.Profiles) != profileCount {
		t.Fatalf("profile count = %d, want %d; concurrent updates were lost", len(state.Profiles), profileCount)
	}
	ids := make(map[string]struct{}, profileCount)
	for _, profile := range state.Profiles {
		if _, exists := ids[profile.ID]; exists {
			t.Fatalf("duplicate generated ID %q", profile.ID)
		}
		ids[profile.ID] = struct{}{}
	}
	if _, err := os.Stat(path + ".lock"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock file remains after concurrent writes: %v", err)
	}
	temporaryFiles, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".profiles-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temporaryFiles) != 0 {
		t.Fatalf("temporary files remain after atomic writes: %v", temporaryFiles)
	}
}

func TestInvalidStateIsClassifiedAndNeverOverwritten(t *testing.T) {
	store, path := testStore(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	invalid := []byte(`{"version":1,"current_profile":"","profiles":[],"api_key":"FAKE_TEST_KEY_DO_NOT_USE"}`)
	if err := os.WriteFile(path, invalid, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := store.Load()
	requireErrorKind(t, err, apperr.KindConfig)
	if strings.Contains(err.Error(), "FAKE_TEST_KEY_DO_NOT_USE") {
		t.Fatalf("configuration error leaked a credential value: %v", err)
	}
	if _, err := store.Create(testProfile("valid")); err == nil {
		t.Fatal("Create overwrote an invalid configuration")
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(invalid, after) {
		t.Fatal("failed operation changed the original configuration")
	}
}
