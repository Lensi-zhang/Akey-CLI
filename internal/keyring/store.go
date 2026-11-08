package keyring

import (
	"runtime"

	systemkeyring "github.com/99designs/keyring"
	"github.com/Lensi-zhang/Akey-CLI/internal/apperr"
	"github.com/Lensi-zhang/Akey-CLI/internal/config"
)

const ServiceName = "com.akey.cli"

type Store interface {
	Set(profile config.Profile, secret string) error
	Get(profile config.Profile) (string, error)
	Delete(profile config.Profile) error
	Status(profile config.Profile) (Status, error)
}

type Status struct {
	Configured bool   `json:"configured"`
	Backend    string `json:"backend"`
}

type backend interface {
	Get(key string) (systemkeyring.Item, error)
	Set(item systemkeyring.Item) error
	Remove(key string) error
}

type openFunc func(systemkeyring.Config) (systemkeyring.Keyring, error)

type System struct {
	ring    backend
	backend string
	goos    string
}

func NewSystem() (*System, error) {
	return newSystem(runtime.GOOS, systemkeyring.Open)
}

func newSystem(goos string, open openFunc) (*System, error) {
	selected, err := backendFor(goos)
	if err != nil {
		return nil, err
	}
	ring, openErr := open(systemkeyring.Config{
		ServiceName:     ServiceName,
		AllowedBackends: []systemkeyring.BackendType{selected},
	})
	if openErr != nil {
		return nil, apperr.NewAuth("keyring_unavailable", "The system keyring is unavailable.", platformHint(goos), redactedCause(openErr))
	}
	return &System{ring: ring, backend: string(selected), goos: goos}, nil
}

func backendFor(goos string) (systemkeyring.BackendType, error) {
	switch goos {
	case "windows":
		return systemkeyring.WinCredBackend, nil
	case "darwin":
		return systemkeyring.KeychainBackend, nil
	case "linux":
		return systemkeyring.SecretServiceBackend, nil
	default:
		return systemkeyring.InvalidBackend, apperr.NewAuth(
			"keyring_unsupported",
			"No supported secure system keyring is available on this platform.",
			"Use Windows, macOS, or Linux with an unlocked system credential service; plaintext file fallback is disabled.",
		)
	}
}

func platformHint(goos string) string {
	switch goos {
	case "windows":
		return "Ensure Windows Credential Manager is available, then try again."
	case "darwin":
		return "Ensure macOS Keychain is available and unlocked, then try again."
	case "linux":
		return "Ensure a Secret Service daemon such as GNOME Keyring is running and unlocked, then try again."
	default:
		return "Use a supported secure system credential service; plaintext file fallback is disabled."
	}
}

func account(profile config.Profile) string {
	return "profile:" + profile.ID
}

func (s *System) ready(profile config.Profile) error {
	if s == nil || s.ring == nil {
		return apperr.NewAuth("keyring_unavailable", "The system keyring is not initialized.", "Initialize a supported system keyring and try again.", ErrUnavailable)
	}
	if profile.ID == "" {
		return apperr.NewAuth("invalid_profile", "The profile has no immutable ID.", "Recreate the profile metadata before managing its key.")
	}
	return nil
}

func (s *System) Set(profile config.Profile, secret string) error {
	if err := s.ready(profile); err != nil {
		return err
	}
	if secret == "" {
		return apperr.NewUsage("empty_key", "API key cannot be empty.", "Enter a non-empty API key using the hidden input prompt.")
	}
	if err := s.ring.Set(systemkeyring.Item{Key: account(profile), Data: []byte(secret)}); err != nil {
		return apperr.NewAuth("keyring_write_failed", "The system keyring could not save the API key.", platformHint(s.goos), redactedCause(err))
	}
	return nil
}

func (s *System) Get(profile config.Profile) (string, error) {
	if err := s.ready(profile); err != nil {
		return "", err
	}
	item, err := s.ring.Get(account(profile))
	if errorsIsNotFound(err) {
		return "", apperr.NewAuth("key_not_configured", "No API key is configured for this profile.", "Run 'akey auth login --profile <name>' to configure a key.", ErrNotConfigured)
	}
	if err != nil {
		return "", apperr.NewAuth("keyring_read_failed", "The system keyring could not read the API key.", platformHint(s.goos), redactedCause(err))
	}
	secret := string(item.Data)
	for i := range item.Data {
		item.Data[i] = 0
	}
	return secret, nil
}

func (s *System) Delete(profile config.Profile) error {
	if err := s.ready(profile); err != nil {
		return err
	}
	if err := s.ring.Remove(account(profile)); err != nil && !errorsIsNotFound(err) {
		return apperr.NewAuth("keyring_delete_failed", "The system keyring could not delete the API key.", platformHint(s.goos), redactedCause(err))
	}
	return nil
}

func (s *System) Status(profile config.Profile) (Status, error) {
	status := Status{}
	if s != nil {
		status.Backend = s.backend
	}
	if err := s.ready(profile); err != nil {
		return status, err
	}
	item, err := s.ring.Get(account(profile))
	if errorsIsNotFound(err) {
		return status, nil
	}
	if err != nil {
		return status, apperr.NewAuth("keyring_status_failed", "The system keyring status is unavailable.", platformHint(s.goos), redactedCause(err))
	}
	for i := range item.Data {
		item.Data[i] = 0
	}
	status.Configured = true
	return status, nil
}
