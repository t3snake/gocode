package core

import (
	"encoding/json/v2"
	"testing"
)

func TestSettingsJSON(t *testing.T) {
	test_cases := []struct {
		name     string
		settings Settings
		want     string
	}{
		{name: "empty", want: `{}`},
		{
			name:     "host only",
			settings: Settings{HostID: "urn:uuid:test"},
			want:     `{"host-id":"urn:uuid:test"}`,
		},
		{
			name: "all fields",
			settings: Settings{
				OpenAIClientID: "client",
				IDTokenHint:    "token",
				HostID:         "host",
			},
			want: `{"openai-client-id":"client","id-token-hint":"token","host-id":"host"}`,
		},
	}

	for _, tt := range test_cases {
		t.Run(tt.name, func(t *testing.T) {
			json_bytes, err := json.Marshal(tt.settings)
			if err != nil {
				t.Fatal(err)
			}
			if string(json_bytes) != tt.want {
				t.Fatalf("got %s, want %s", json_bytes, tt.want)
			}

			var parsed_settings Settings
			if err := json.Unmarshal([]byte(tt.want), &parsed_settings); err != nil {
				t.Fatal(err)
			}
			if parsed_settings != tt.settings {
				t.Fatalf("got %+v, want %+v", parsed_settings, tt.settings)
			}
		})
	}
}
