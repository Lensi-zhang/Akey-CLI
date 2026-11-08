package config

import "time"

const (
	CurrentVersion   = 1
	DefaultEnvPrefix = "API"
)

type Profile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Provider  string    `json:"provider"`
	BaseURL   string    `json:"base_url"`
	Model     string    `json:"model"`
	EnvPrefix string    `json:"env_prefix"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type State struct {
	Version        int       `json:"version"`
	CurrentProfile string    `json:"current_profile"`
	Profiles       []Profile `json:"profiles"`
}

// Patch 使用指针区分未提供字段和显式设置的零值。
type Patch struct {
	Provider  *string `json:"provider,omitempty"`
	BaseURL   *string `json:"base_url,omitempty"`
	Model     *string `json:"model,omitempty"`
	EnvPrefix *string `json:"env_prefix,omitempty"`
	Note      *string `json:"note,omitempty"`
}

func emptyState() State {
	return State{Version: CurrentVersion, Profiles: make([]Profile, 0)}
}
