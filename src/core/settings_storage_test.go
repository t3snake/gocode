package core

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSettingsFailedWriteKeepsSavedAccount(t *testing.T) {
	config_dir := t.TempDir()
	t.Setenv("HOME", config_dir)
	t.Setenv("XDG_CONFIG_HOME", config_dir)
	t.Setenv("AppData", config_dir)
	old_settings, old_loaded := settings, is_settings_loaded
	defer func() { settings, is_settings_loaded = old_settings, old_loaded }()

	config_path, err := GetOrInitConfig()
	if err != nil {
		t.Fatal(err)
	}
	// Start with a settings file in the old format and with broad permissions.
	legacy := `{"openai-client-id":"client","host-id":"host","id-token-hint":"old-token"}`
	if err := os.WriteFile(config_path, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	if err := LoadSettings(); err != nil {
		t.Fatal(err)
	}
	account, err := GetSettings()
	if err != nil || account.IDTokenHint != "old-token" {
		t.Fatalf("old ID-token hint was not loaded: %v", err)
	}
	account.AccessToken = "saved-token"
	if err := SaveSettings(account); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(config_path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("credentials permissions = %o, want 600", info.Mode().Perm())
	}

	// Block the final rename after the temporary credentials file has been written.
	backup_path := config_path + ".backup"
	if err := os.Rename(config_path, backup_path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(config_path, 0700); err != nil {
		t.Fatal(err)
	}
	replacement := account
	replacement.AccessToken = "unsaved-token"
	if err := SaveSettings(replacement); err == nil {
		t.Fatal("save succeeded with a directory in place of the file")
	}
	cached, err := GetSettings()
	if err != nil || cached != account {
		t.Fatalf("failed save changed the cached account: %v", err)
	}
	temp_files, err := filepath.Glob(filepath.Join(filepath.Dir(config_path), ".settings-*"))
	if err != nil || len(temp_files) != 0 {
		t.Fatalf("failed save left temporary credentials: %v", err)
	}
	if err := os.Remove(config_path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backup_path, config_path); err != nil {
		t.Fatal(err)
	}
	if err := LoadSettings(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := GetSettings()
	if err != nil || reloaded != account {
		t.Fatalf("saved account changed after failed write: %v", err)
	}
}
