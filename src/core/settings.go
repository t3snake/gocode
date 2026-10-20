package core

import (
	"encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Settings holds the values stored in the config file.
type Settings struct {
	OpenAIClientID        string    `json:"openai-client-id,omitempty"`
	IDTokenHint           string    `json:"id-token-hint,omitempty"`
	PendingOpenAIClientID string    `json:"pending-openai-client-id,omitempty"`
	HostID                string    `json:"host-id,omitempty"`
	Subject               string    `json:"subject,omitempty"`
	Email                 string    `json:"email,omitempty"`
	AccessToken           string    `json:"access-token,omitempty"`
	RefreshToken          string    `json:"refresh-token,omitempty"`
	Scope                 string    `json:"scope,omitempty"`
	ExpiresAt             time.Time `json:"expires-at,omitzero"`
}

// Globals

// is_settings_loaded indicates if the settings were already read from the config
var is_settings_loaded bool = false

// settings represents the settings read from the config file
var settings Settings

// LoadSettings will read the config json and load the settings. Also see [GetSettings].
// A missing file loads empty settings. Other errors keep the previous settings.
//
// This function does not create a "settings.json" if the file is not present. That is done in [SaveSettings]
func LoadSettings() error {
	var new_settings Settings

	config_file_path, err := GetOrInitConfig()
	if err != nil {
		return err
	}

	content, err := os.ReadFile(config_file_path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err == nil {
		if err := json.Unmarshal(content, &new_settings); err != nil {
			return err
		}
	}

	is_settings_loaded = true
	settings = new_settings
	return nil
}

// GetSettings will return a copy of the settings from the parsed config json.
// If the settings were never read from the config json, this will read it using [LoadSettings].
func GetSettings() (Settings, error) {
	if !is_settings_loaded {
		err := LoadSettings()
		if err != nil {
			return Settings{}, err
		}
	}

	return settings, nil
}

func SaveSettings(new_settings Settings) error {
	config_file_path, err := GetOrInitConfig()
	if err != nil {
		return err
	}

	json_bytes, err := json.Marshal(new_settings)
	if err != nil {
		return err
	}

	temp_file, err := os.CreateTemp(filepath.Dir(config_file_path), ".settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp_file.Name())
	_, write_err := temp_file.Write(json_bytes)
	close_err := temp_file.Close()
	if write_err != nil {
		return write_err
	}
	if close_err != nil {
		return close_err
	}
	if err := os.Rename(temp_file.Name(), config_file_path); err != nil {
		return err
	}

	settings = new_settings // only replace if settings was successfully written, else keep the old value
	is_settings_loaded = true
	return nil
}

// GetOrInitConfig returns the path to the config/settings file.
// This will create gocode directory in User config folder if it does not exist.
func GetOrInitConfig() (string, error) {
	usr_conf_dir, err := os.UserConfigDir()
	if err != nil {
		// default settings
		return "", err
	}

	gc_conf_dir := filepath.Join(usr_conf_dir, "gocode")
	if err := os.MkdirAll(gc_conf_dir, 0700); err != nil {
		return "", err
	}

	return filepath.Join(gc_conf_dir, "settings.json"), nil
}
