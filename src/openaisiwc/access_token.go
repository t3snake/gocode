package openaisiwc

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/t3snake/gocode/src/core"
)

// Serialize token renewal within this process. Run one process per saved session.
var refresh_mutex sync.Mutex

// GetAccessToken reads the saved SIWC credentials and renews an expiring token.
// The client ID belongs in the OAuth refresh request, not the inference request.
func GetAccessToken(ctx context.Context) (string, error) {
	refresh_mutex.Lock()
	defer refresh_mutex.Unlock()

	if err := ctx.Err(); err != nil {
		return "", err
	}
	settings, err := core.GetSettings()
	if err != nil {
		return "", fmt.Errorf("read SIWC settings: %w", err)
	}
	if settings.OpenAIClientID == "" || settings.AccessToken == "" {
		return "", fmt.Errorf("SIWC credentials are missing; run gocode -login")
	}
	if !PlanUsageEnabled(settings.Scope) {
		return "", fmt.Errorf("ChatGPT plan usage was not granted; run gocode -login")
	}
	if time.Now().Add(time.Minute).Before(settings.ExpiresAt) {
		return settings.AccessToken, nil
	}
	if settings.RefreshToken == "" {
		return "", fmt.Errorf("SIWC token needs renewal but no refresh token is saved; run gocode -login")
	}

	refresh_ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	request_time := time.Now()
	tokens, err := exchangeCode(refresh_ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {settings.OpenAIClientID},
		"refresh_token": {settings.RefreshToken},
		"resource":      {"https://api.openai.com/v1"},
	})
	if err != nil {
		return "", fmt.Errorf("renew SIWC token: %w", err)
	}
	if tokens.RefreshToken == "" {
		return "", fmt.Errorf("SIWC renewal returned no replacement refresh token; run gocode -login")
	}
	settings.AccessToken = tokens.AccessToken
	settings.RefreshToken = tokens.RefreshToken
	settings.ExpiresAt = request_time.Add(time.Duration(tokens.ExpiresIn) * time.Second)
	if tokens.Scope != "" {
		settings.Scope = tokens.Scope
	}
	// Keep the identity and ID-token hint validated during sign-in.
	if err := core.SaveSettings(settings); err != nil {
		return "", fmt.Errorf("save renewed SIWC credentials: %w", err)
	}
	if !PlanUsageEnabled(settings.Scope) {
		return "", fmt.Errorf("ChatGPT plan usage was not granted after token renewal")
	}
	return settings.AccessToken, nil
}
