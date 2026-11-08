package config

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Lensi-zhang/Akey-CLI/internal/apperr"
	"github.com/Lensi-zhang/Akey-CLI/internal/validate"
)

const (
	lockRetryDelay = 10 * time.Millisecond
	lockTimeout    = 30 * time.Second
)

type Store struct {
	path    string
	initErr error
}

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".akey", "profiles.json"), nil
}

// NewStore 在不传路径或传入空路径时使用默认路径，自定义路径仅用于调用方显式隔离存储。
func NewStore(paths ...string) *Store {
	store := &Store{}
	if len(paths) > 1 {
		store.initErr = errors.New("NewStore accepts at most one path")
		return store
	}

	var path string
	if len(paths) == 1 {
		path = paths[0]
	}
	if path == "" {
		path, store.initErr = DefaultPath()
	} else {
		path = filepath.Clean(path)
	}
	store.path = path
	return store
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) Load() (State, error) {
	if err := s.ready(); err != nil {
		return State{}, err
	}
	var state State
	err := s.withLock(func() error {
		var err error
		state, err = s.load()
		return err
	})
	return state, err
}

func (s *Store) ready() error {
	if s == nil {
		return apperr.NewConfig("config_unavailable", "Configuration store is not initialized.", "Initialize a configuration store and try again.")
	}
	if s.initErr != nil {
		return apperr.NewConfig("config_path_failed", "The configuration path could not be resolved.", "Check the home directory configuration and try again.", s.initErr)
	}
	if s.path == "" {
		return apperr.NewConfig("config_path_failed", "The configuration path is empty.", "Provide a configuration file path and try again.")
	}
	return nil
}

func (s *Store) load() (State, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return emptyState(), nil
	}
	if err != nil {
		return State{}, apperr.NewConfig("config_read_failed", "The profile configuration could not be read.", "Check the configuration file permissions and try again.", err)
	}

	state := emptyState()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return State{}, invalidState(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return State{}, invalidState(err)
	}
	if state.Profiles == nil {
		state.Profiles = make([]Profile, 0)
	}
	if err := validateState(&state); err != nil {
		return State{}, err
	}
	return state, nil
}

func invalidState(cause error) error {
	return apperr.NewConfig("invalid_config", "The profile configuration is invalid.", "Repair or remove the invalid configuration before trying again.", cause)
}

func validateState(state *State) error {
	if state.Version != CurrentVersion {
		return apperr.NewConfig("unsupported_config_version", "The profile configuration version is not supported.", "Use a compatible akey version or migrate the configuration explicitly.")
	}

	names := make(map[string]struct{}, len(state.Profiles))
	ids := make(map[string]struct{}, len(state.Profiles))
	for i := range state.Profiles {
		profile := &state.Profiles[i]
		if profile.EnvPrefix == "" {
			profile.EnvPrefix = DefaultEnvPrefix
		}
		if err := validateMetadata(*profile); err != nil {
			return err
		}
		if !validID(profile.ID) {
			return invalidState(errors.New("invalid profile id"))
		}
		if profile.CreatedAt.IsZero() || profile.UpdatedAt.IsZero() || !isUTC(profile.CreatedAt) || !isUTC(profile.UpdatedAt) {
			return invalidState(errors.New("profile timestamps must be UTC RFC3339 timestamps"))
		}
		if _, exists := names[profile.Name]; exists {
			return invalidState(errors.New("duplicate profile name"))
		}
		if _, exists := ids[profile.ID]; exists {
			return invalidState(errors.New("duplicate profile id"))
		}
		names[profile.Name] = struct{}{}
		ids[profile.ID] = struct{}{}
	}
	if state.CurrentProfile != "" {
		if _, exists := names[state.CurrentProfile]; !exists {
			return invalidState(errors.New("current profile does not exist"))
		}
	}
	return nil
}

func validateMetadata(profile Profile) error {
	if err := validate.ProfileName(profile.Name); err != nil {
		return err
	}
	if strings.TrimSpace(profile.Provider) == "" {
		return apperr.NewConfig("invalid_provider", "Profile provider is required.", "Provide a non-empty provider.")
	}
	if err := validate.BaseURL(profile.BaseURL); err != nil {
		return err
	}
	if strings.TrimSpace(profile.Model) == "" {
		return apperr.NewConfig("invalid_model", "Profile model is required.", "Provide a non-empty model or endpoint identifier.")
	}
	return validate.EnvPrefix(profile.EnvPrefix)
}

func validID(id string) bool {
	if len(id) != 32 || id != strings.ToLower(id) {
		return false
	}
	decoded, err := hex.DecodeString(id)
	return err == nil && len(decoded) == 16
}

func isUTC(value time.Time) bool {
	_, offset := value.Zone()
	return offset == 0
}

func (s *Store) Create(profile Profile) (Profile, error) {
	if err := s.ready(); err != nil {
		return Profile{}, err
	}
	id, err := newID()
	if err != nil {
		return Profile{}, apperr.NewRuntime("id_generation_failed", "A random profile ID could not be generated.", "Try the operation again.", err)
	}

	var created Profile
	err = s.mutate(func(state *State) error {
		if findProfile(state, profile.Name) >= 0 {
			return profileConflict(profile.Name)
		}
		if profile.EnvPrefix == "" {
			profile.EnvPrefix = DefaultEnvPrefix
		}
		now := time.Now().UTC().Truncate(time.Second)
		profile.ID = id
		profile.CreatedAt = now
		profile.UpdatedAt = now
		if err := validateMetadata(profile); err != nil {
			return err
		}
		state.Profiles = append(state.Profiles, profile)
		created = profile
		return nil
	})
	return created, err
}

func newID() (string, error) {
	var random [16]byte
	if _, err := io.ReadFull(rand.Reader, random[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(random[:]), nil
}

func (s *Store) Get(name string) (Profile, error) {
	state, err := s.Load()
	if err != nil {
		return Profile{}, err
	}
	index := findProfile(&state, name)
	if index < 0 {
		return Profile{}, profileNotFound(name)
	}
	return state.Profiles[index], nil
}

func (s *Store) List() ([]Profile, error) {
	state, err := s.Load()
	if err != nil {
		return nil, err
	}
	return state.Profiles, nil
}

func (s *Store) Update(name string, patch Patch) (Profile, error) {
	if err := s.ready(); err != nil {
		return Profile{}, err
	}
	var updated Profile
	err := s.mutate(func(state *State) error {
		index := findProfile(state, name)
		if index < 0 {
			return profileNotFound(name)
		}
		profile := &state.Profiles[index]
		if patch.Provider != nil {
			profile.Provider = *patch.Provider
		}
		if patch.BaseURL != nil {
			profile.BaseURL = *patch.BaseURL
		}
		if patch.Model != nil {
			profile.Model = *patch.Model
		}
		if patch.EnvPrefix != nil {
			profile.EnvPrefix = *patch.EnvPrefix
			if profile.EnvPrefix == "" {
				profile.EnvPrefix = DefaultEnvPrefix
			}
		}
		if patch.Note != nil {
			profile.Note = *patch.Note
		}
		profile.UpdatedAt = time.Now().UTC().Truncate(time.Second)
		if err := validateMetadata(*profile); err != nil {
			return err
		}
		updated = *profile
		return nil
	})
	return updated, err
}

func (s *Store) Rename(oldName, newName string) (Profile, error) {
	if err := s.ready(); err != nil {
		return Profile{}, err
	}
	if err := validate.ProfileName(newName); err != nil {
		return Profile{}, err
	}
	var renamed Profile
	err := s.mutate(func(state *State) error {
		index := findProfile(state, oldName)
		if index < 0 {
			return profileNotFound(oldName)
		}
		if oldName != newName && findProfile(state, newName) >= 0 {
			return profileConflict(newName)
		}
		profile := &state.Profiles[index]
		profile.Name = newName
		profile.UpdatedAt = time.Now().UTC().Truncate(time.Second)
		if state.CurrentProfile == oldName {
			state.CurrentProfile = newName
		}
		renamed = *profile
		return nil
	})
	return renamed, err
}

func (s *Store) Use(name string) error {
	if err := s.ready(); err != nil {
		return err
	}
	return s.mutate(func(state *State) error {
		if findProfile(state, name) < 0 {
			return profileNotFound(name)
		}
		state.CurrentProfile = name
		return nil
	})
}

func (s *Store) Current() (Profile, error) {
	state, err := s.Load()
	if err != nil {
		return Profile{}, err
	}
	if state.CurrentProfile == "" {
		return Profile{}, apperr.NewNotFound("no_current_profile", "No default profile is configured.", "Run 'akey profile use <name>' to select a default profile.")
	}
	index := findProfile(&state, state.CurrentProfile)
	if index < 0 {
		return Profile{}, invalidState(errors.New("current profile does not exist"))
	}
	return state.Profiles[index], nil
}

func (s *Store) Delete(name string) error {
	if err := s.ready(); err != nil {
		return err
	}
	return s.mutate(func(state *State) error {
		index := findProfile(state, name)
		if index < 0 {
			return profileNotFound(name)
		}
		copy(state.Profiles[index:], state.Profiles[index+1:])
		state.Profiles[len(state.Profiles)-1] = Profile{}
		state.Profiles = state.Profiles[:len(state.Profiles)-1]
		if state.CurrentProfile == name {
			state.CurrentProfile = ""
		}
		return nil
	})
}

func findProfile(state *State, name string) int {
	for i := range state.Profiles {
		if state.Profiles[i].Name == name {
			return i
		}
	}
	return -1
}

func profileNotFound(name string) error {
	return apperr.NewNotFound("profile_not_found", fmt.Sprintf("Profile %q does not exist.", name), "Run 'akey profile list' to view available profiles.")
}

func profileConflict(name string) error {
	return apperr.NewConflict("profile_exists", fmt.Sprintf("Profile %q already exists.", name), "Choose a different profile name.")
}

func (s *Store) mutate(change func(*State) error) error {
	return s.withLock(func() error {
		state, err := s.load()
		if err != nil {
			return err
		}
		if err := change(&state); err != nil {
			return err
		}
		if err := validateState(&state); err != nil {
			return err
		}
		return s.save(state)
	})
}

func (s *Store) withLock(operation func() error) error {
	if err := s.ready(); err != nil {
		return err
	}
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return apperr.NewConfig("config_directory_failed", "The configuration directory could not be created.", "Check the parent directory permissions and try again.", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return apperr.NewConfig("config_directory_failed", "The configuration directory permissions could not be secured.", "Set the configuration directory permissions to 0700 and try again.", err)
	}

	lockPath := s.path + ".lock"
	deadline := time.Now().Add(lockTimeout)
	for {
		lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			if closeErr := lock.Close(); closeErr != nil {
				_ = os.Remove(lockPath)
				return apperr.NewConfig("config_lock_failed", "The configuration lock could not be initialized.", "Try the operation again.", closeErr)
			}
			defer os.Remove(lockPath)
			return operation()
		}
		if !isLockContention(err) {
			return apperr.NewConfig("config_lock_failed", "The configuration lock could not be created.", "Check the configuration directory permissions and try again.", err)
		}
		if time.Now().After(deadline) {
			return apperr.NewConfig("config_lock_timeout", "Timed out waiting for the configuration lock.", "Wait for other akey processes to finish; remove the lock file only if it is stale.")
		}
		time.Sleep(lockRetryDelay)
	}
}

func (s *Store) save(state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return apperr.NewConfig("config_encode_failed", "The profile configuration could not be encoded.", "Try the operation again.", err)
	}
	data = append(data, '\n')

	directory := filepath.Dir(s.path)
	temporary, err := os.CreateTemp(directory, ".profiles-*.tmp")
	if err != nil {
		return apperr.NewConfig("config_write_failed", "A temporary configuration file could not be created.", "Check the configuration directory permissions and free disk space.", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()

	if err := temporary.Chmod(0o600); err != nil {
		return apperr.NewConfig("config_write_failed", "The temporary configuration file permissions could not be secured.", "Set the configuration file permissions to 0600 and try again.", err)
	}
	written, err := temporary.Write(data)
	if err == nil && written != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return apperr.NewConfig("config_write_failed", "The temporary configuration file could not be written.", "Check free disk space and try again.", err)
	}
	if err := temporary.Sync(); err != nil {
		return apperr.NewConfig("config_write_failed", "The temporary configuration file could not be flushed.", "Check the storage device and try again.", err)
	}
	if err := temporary.Close(); err != nil {
		return apperr.NewConfig("config_write_failed", "The temporary configuration file could not be closed.", "Check the storage device and try again.", err)
	}
	if err := atomicReplace(temporaryPath, s.path); err != nil {
		return apperr.NewConfig("config_replace_failed", "The profile configuration could not be replaced atomically.", "The previous configuration was preserved; try the operation again.", err)
	}
	return nil
}
