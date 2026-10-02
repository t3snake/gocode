package core

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"os"
)

// Globals

// is_settings_loaded indicates if the settings were already read from the config
var is_settings_loaded bool = false

// settings represents the settings read from the config
var settings map[string]any

// LoadSettings will read the config json and load the settings map. Also see [GetSettings].
func LoadSettings() error {
	// realloc settings whether already loaded or not
	settings = make(map[string]any)

	config_file_path, err := GetOrInitConfig()
	if err != nil {
		return err
	}

	content, err := os.ReadFile(config_file_path)
	if err != nil {
		return err
	}

	err = json.Unmarshal(content, settings)
	if err != nil {
		return err
	}

	is_settings_loaded = true
	return nil
}

// GetSettings will return the map of settings from the parsed config json.
// If the settings were never read from the config json, this will read it using [LoadSettings].
func GetSettings() (map[string]any, error) {
	var copy map[string]any

	if !is_settings_loaded {
		err := LoadSettings()
		if err != nil {
			return copy, err
		}
	}

	maps.Copy(copy, settings)

	return copy, nil
}

func SaveSettings(new_settings map[string]any) error {
	config_file_path, err := GetOrInitConfig()
	if err != nil {
		return err
	}

	json_bytes, err := json.Marshal(new_settings)

	err = os.WriteFile(config_file_path, json_bytes, 0666)
	if err != nil {
		return err
	}

	settings = new_settings // only replace if settings was successfully written, else keep the old value
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

	gc_conf_dir := fmt.Sprintf("%s/%s", usr_conf_dir, "gocode")

	_, err = os.Stat(gc_conf_dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err2 := os.Mkdir(gc_conf_dir, 0700)
			if err2 != nil {
				return "", err2
			}
		} else {
			return "", err
		}
	}

	return fmt.Sprintf("%s/%s", gc_conf_dir, "settings.json"), nil
}
