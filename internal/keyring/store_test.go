package keyring

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	systemkeyring "github.com/99designs/keyring"
	"github.com/Lensi-zhang/Akey-CLI/internal/apperr"
	"github.com/Lensi-zhang/Akey-CLI/internal/config"
)

type fakeBackend struct {
	mutex sync.Mutex
	items map[string]systemkeyring.Item
	err   error
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{items: make(map[string]systemkeyring.Item)}
}

func (f *fakeBackend) Get(key string) (systemkeyring.Item, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if f.err != nil {
		return systemkeyring.Item{}, f.err
	}
	item, exists := f.items[key]
	if !exists {
		return systemkeyring.Item{}, systemkeyring.ErrKeyNotFound
	}
	item.Data = bytes.Clone(item.Data)
	return item, nil
}

func (f *fakeBackend) Set(item systemkeyring.Item) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if f.err != nil {
		return f.err
	}
	item.Data = bytes.Clone(item.Data)
	f.items[item.Key] = item
	return nil
}

func (f *fakeBackend) Remove(key string) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if f.err != nil {
		return f.err
	}
	if _, exists := f.items[key]; !exists {
		return systemkeyring.ErrKeyNotFound
	}
	delete(f.items, key)
	return nil
}

func testProfile() config.Profile {
	return config.Profile{ID: "0123456789abcdef0123456789abcdef", Name: "original-name"}
}

func TestBackendSelection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		goos string
		want systemkeyring.BackendType
	}{
		{goos: "windows", want: systemkeyring.WinCredBackend},
		{goos: "darwin", want: systemkeyring.KeychainBackend},
		{goos: "linux", want: systemkeyring.SecretServiceBackend},
	}
	for _, test := range tests {
		t.Run(test.goos, func(t *testing.T) {
			got, err := backendFor(test.goos)
			if err != nil {
				t.Fatalf("backendFor: %v", err)
			}
			if got != test.want || got == systemkeyring.FileBackend {
				t.Fatalf("backend = %q, want secure backend %q", got, test.want)
			}
		})
	}
	for _, goos := range []string{"freebsd", "openbsd", "plan9", "js", ""} {
		if backend, err := backendFor(goos); err == nil {
			t.Errorf("backendFor(%q) unexpectedly returned %q", goos, backend)
		} else {
			requireKind(t, err, apperr.KindAuth)
		}
	}
}

func TestNewSystemRestrictsBackendAndService(t *testing.T) {
	t.Parallel()
	for _, goos := range []string{"windows", "darwin", "linux"} {
		t.Run(goos, func(t *testing.T) {
			want, err := backendFor(goos)
			if err != nil {
				t.Fatal(err)
			}
			var captured systemkeyring.Config
			system, err := newSystem(goos, func(configuration systemkeyring.Config) (systemkeyring.Keyring, error) {
				captured = configuration
				return nil, errors.New("injected opener failure")
			})
			if system != nil || err == nil {
				t.Fatal("newSystem succeeded despite injected opener failure")
			}
			if captured.ServiceName != ServiceName {
				t.Fatalf("ServiceName = %q, want %q", captured.ServiceName, ServiceName)
			}
			if len(captured.AllowedBackends) != 1 || captured.AllowedBackends[0] != want || captured.AllowedBackends[0] == systemkeyring.FileBackend {
				t.Fatalf("AllowedBackends = %v, want only %q", captured.AllowedBackends, want)
			}
			if captured.FileDir != "" || captured.FilePasswordFunc != nil {
				t.Fatal("file backend configuration was unexpectedly enabled")
			}
		})
	}

	called := false
	if _, err := newSystem("plan9", func(systemkeyring.Config) (systemkeyring.Keyring, error) {
		called = true
		return nil, nil
	}); err == nil || called {
		t.Fatal("unsupported platform attempted to open a backend or succeeded")
	}
}

func TestSystemUsesImmutableProfileID(t *testing.T) {
	backend := newFakeBackend()
	system := &System{ring: backend, backend: string(systemkeyring.WinCredBackend), goos: "windows"}
	profile := testProfile()
	fakeSecret := "FAKE_TEST_KEY_DO_NOT_USE_19e28a"

	if err := system.Set(profile, fakeSecret); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, exists := backend.items["profile:"+profile.ID]; !exists {
		t.Fatal("Set did not use profile:<id> as the account")
	}
	if _, exists := backend.items["profile:"+profile.Name]; exists {
		t.Fatal("Set used the mutable profile name as the account")
	}

	profile.Name = "renamed-profile"
	got, err := system.Get(profile)
	if err != nil || got != fakeSecret {
		t.Fatalf("Get after rename = %q, %v", got, err)
	}
	status, err := system.Status(profile)
	if err != nil || !status.Configured || status.Backend != string(systemkeyring.WinCredBackend) {
		t.Fatalf("Status = %#v, %v", status, err)
	}
	statusJSON, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(statusJSON, []byte(fakeSecret)) {
		t.Fatalf("Status JSON leaked a key: %s", statusJSON)
	}

	if err := system.Delete(profile); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	status, err = system.Status(profile)
	if err != nil || status.Configured {
		t.Fatalf("Status after Delete = %#v, %v", status, err)
	}
	if _, err := system.Get(profile); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Get after Delete error = %v, want ErrNotConfigured", err)
	} else {
		requireKind(t, err, apperr.KindAuth)
	}
	if err := system.Delete(profile); err != nil {
		t.Fatalf("Delete missing key should be idempotent: %v", err)
	}
}

func TestThirdPartyErrorsNeverLeakFakeSecret(t *testing.T) {
	const fakeSecret = "FAKE_TEST_KEY_DO_NOT_USE_a7c913"
	backend := newFakeBackend()
	backend.err = fmt.Errorf("backend failure included %s", fakeSecret)
	system := &System{ring: backend, backend: string(systemkeyring.SecretServiceBackend), goos: "linux"}
	profile := testProfile()
	secret := fakeSecret

	var operationErrors []error
	operationErrors = append(operationErrors, system.Set(profile, secret))
	_, getErr := system.Get(profile)
	operationErrors = append(operationErrors, getErr)
	operationErrors = append(operationErrors, system.Delete(profile))
	_, statusErr := system.Status(profile)
	operationErrors = append(operationErrors, statusErr)
	_, openErr := newSystem("linux", func(systemkeyring.Config) (systemkeyring.Keyring, error) {
		return nil, fmt.Errorf("open failure included %s", fakeSecret)
	})
	operationErrors = append(operationErrors, openErr)

	for _, err := range operationErrors {
		if err == nil {
			t.Fatal("injected backend failure unexpectedly returned nil")
		}
		requireKind(t, err, apperr.KindAuth)
		for current := err; current != nil; current = errors.Unwrap(current) {
			if strings.Contains(current.Error(), fakeSecret) || strings.Contains(current.Error(), "backend failure") || strings.Contains(current.Error(), "open failure") {
				t.Fatalf("error chain leaked third-party text: %v", current)
			}
		}
		encoded, marshalErr := json.Marshal(err)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatalf("error JSON leaked a key: %s", encoded)
		}
	}
}

func TestEmptyKeyAndUninitializedSystemFailSafely(t *testing.T) {
	t.Parallel()
	profile := testProfile()
	system := &System{ring: newFakeBackend(), backend: string(systemkeyring.WinCredBackend), goos: "windows"}
	if err := system.Set(profile, ""); err == nil {
		t.Fatal("Set accepted an empty key")
	} else {
		requireKind(t, err, apperr.KindUsage)
	}
	var uninitialized *System
	if _, err := uninitialized.Get(profile); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("uninitialized Get error = %v, want ErrUnavailable", err)
	}
	profile.ID = ""
	if err := system.Set(profile, "FAKE_TEST_KEY_DO_NOT_USE"); err == nil {
		t.Fatal("Set accepted a profile without an immutable ID")
	}
}

func requireKind(t *testing.T, err error, want apperr.Kind) {
	t.Helper()
	var applicationError *apperr.Error
	if !errors.As(err, &applicationError) {
		t.Fatalf("error %v is not an apperr.Error", err)
	}
	if applicationError.Kind != want {
		t.Fatalf("error kind = %q, want %q", applicationError.Kind, want)
	}
}
