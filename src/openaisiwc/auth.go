package openaisiwc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/t3snake/gocode/src/core"
)

// LoginChatGPT signs in one account. It does not start an inference request.
func LoginChatGPT(ctx context.Context, out io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	settings, err := core.GetSettings()
	if err != nil {
		return err
	}
	settings.HostID, err = getAndPersistHostId(settings)
	if err != nil {
		return err
	}

	state := rand.Text()
	listener, err := startCallbackListener(state, getStoredClientId(settings))
	if err != nil {
		return err
	}
	defer func() {
		shutdown_ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		listener.server.Shutdown(shutdown_ctx)
		listener.server.Close()
	}()
	params := url.Values{}
	verifier := fillQueryParams(params, settings, listener.uri, state)
	sign_in_uri := core.OpenAiSiwcAuthEndpoint + "?" + params.Encode()
	// OSC 8 sets the target; SGR adds a visible underline and color.
	// Keep the plain URL separate for terminals that do not support hyperlinks.
	_, err = fmt.Fprintf(out, "\x1b]8;;%s\x1b\\\x1b[4;36mContinue with ChatGPT\x1b[0m\x1b]8;;\x1b\\\n\nIf the link does not open, copy this URL into your browser:\n%s\n", sign_in_uri, sign_in_uri)
	if err != nil {
		return fmt.Errorf("print sign-in link: %w", err)
	}

	var result callbackResult
	select {
	case <-ctx.Done():
		return ctx.Err()
	case result = <-listener.results:
	}
	if result.err != nil {
		return result.err
	}
	client_id := result.client_id
	// Keep a new registration for retries, but do not activate unverified credentials.
	if settings.OpenAIClientID == "" && settings.PendingOpenAIClientID == "" {
		settings.PendingOpenAIClientID = client_id
		if err := core.SaveSettings(settings); err != nil {
			return err
		}
	}

	tokens, err := exchangeCode(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {client_id},
		"code":          {result.code},
		"code_verifier": {verifier},
		"redirect_uri":  {listener.uri},
		"resource":      {"https://api.openai.com/v1"},
	})
	if err != nil {
		return err
	}
	expires_at := time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
	signing_keys, err := fetchSigningKeys(ctx)
	if err != nil {
		return err
	}
	identity, err := validateIDToken(tokens.IDToken, signing_keys, client_id, params.Get("nonce"), time.Now())
	if err != nil {
		return err
	}
	if settings.Subject != "" && identity.Subject != settings.Subject {
		return fmt.Errorf("signed-in account does not match the saved account")
	}
	settings.OpenAIClientID = client_id
	settings.PendingOpenAIClientID = ""
	settings.Subject = identity.Subject
	settings.Email = identity.Email
	settings.IDTokenHint = tokens.IDToken
	settings.AccessToken = tokens.AccessToken
	settings.RefreshToken = tokens.RefreshToken
	settings.Scope = tokens.Scope
	settings.ExpiresAt = expires_at
	return core.SaveSettings(settings)
}

// getStoredClientId also returns an unfinished registration so a retry can use it.
func getStoredClientId(settings core.Settings) string {
	if settings.OpenAIClientID != "" {
		return settings.OpenAIClientID
	}
	return settings.PendingOpenAIClientID
}

func fillQueryParams(params url.Values, settings core.Settings, callback_uri, state string) string {
	client_id := getStoredClientId(settings)
	if client_id == "" {
		params.Set("client_id", "dynamic_agent_client")
		params.Set("agent_name_hint", "gocode")
	} else {
		params.Set("client_id", client_id)
		if settings.OpenAIClientID != "" && settings.IDTokenHint != "" {
			params.Set("id_token_hint", settings.IDTokenHint)
		}
	}

	params.Set("ext_agent_host_id", settings.HostID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", callback_uri)
	params.Set("scope", "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct")
	params.Set("resource", "https://api.openai.com/v1")
	params.Set("state", state)
	params.Set("nonce", rand.Text())
	params.Set("code_challenge_method", "S256")

	// Hash the encoded verifier: this exact string is sent during token exchange.
	verifier_bytes := make([]byte, 32)
	rand.Read(verifier_bytes)
	verifier := base64.RawURLEncoding.EncodeToString(verifier_bytes)
	challenge := sha256.Sum256([]byte(verifier))
	params.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	return verifier
}

func getAndPersistHostId(settings core.Settings) (string, error) {
	if settings.HostID != "" {
		return settings.HostID, nil
	}
	settings.HostID = "urn:uuid:" + uuid.NewV4().String()
	err := core.SaveSettings(settings)
	return settings.HostID, err
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
}

func exchangeCode(ctx context.Context, params url.Values) (tokenResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, core.OpenAiSiwcTokenEndpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return tokenResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var oauth_error struct {
			Code string `json:"error"`
		}
		json.UnmarshalRead(io.LimitReader(response.Body, 1<<20), &oauth_error)
		return tokenResponse{}, fmt.Errorf("token exchange: HTTP %d (%q)", response.StatusCode, oauth_error.Code)
	}
	var tokens tokenResponse
	if err := json.UnmarshalRead(io.LimitReader(response.Body, 1<<20), &tokens); err != nil {
		return tokenResponse{}, fmt.Errorf("decode tokens: %w", err)
	}
	if tokens.AccessToken == "" || tokens.IDToken == "" || !strings.EqualFold(tokens.TokenType, "Bearer") {
		return tokenResponse{}, fmt.Errorf("token exchange returned incomplete credentials")
	}
	// Check before converting seconds to time.Duration, which stores nanoseconds.
	if tokens.ExpiresIn <= 0 || tokens.ExpiresIn > int64((1<<63-1)/time.Second) {
		return tokenResponse{}, fmt.Errorf("token exchange returned an invalid expiry")
	}
	return tokens, nil
}

// PlanUsageEnabled checks the granted scopes, not the requested scopes.
func PlanUsageEnabled(scope string) bool {
	return slices.Contains(strings.Fields(scope), "chatgpt.tokens.use.direct")
}
