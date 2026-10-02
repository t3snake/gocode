package openaisiwc

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"uuid"

	"github.com/t3snake/gocode/src/core"
)

func LoginChatGPT() (retcode int) {
	retcode = 0

	var query_params map[string]string

	settings, err := core.GetSettings()
	if err != nil {
		return 1
	}

	callback_uri := StartListeningForCallback()

	fillQueryParams(query_params, settings, callback_uri)

	return 0
}

// getStoredClientId returns a stored openai issued client id, if this is a new logon, returns empty string
func getStoredClientId(settings map[string]any) string {
	id, ok := settings["openai-client-id"]
	if !ok {
		return ""
	}

	id_str, ok := id.(string)
	if ok {
		return id_str
	}

	return ""
}

func fillQueryParams(params map[string]string, settings map[string]any, callback_uri string) (pkce_verifier []byte) {
	client_id := getStoredClientId(settings)
	is_new_flow := client_id == ""

	if is_new_flow {
		// Client Id
		params["client_id"] = "dynamic_agent_client"

		// Agent Name Hint - only for new logon
		params["agent_name_hint"] = "gocode"

	} else {
		// Client Id
		params["client_id"] = client_id

		// ID Token Hint - only if present in settings or skip
		token_hint, ok := settings["id-token-hint"]
		if ok {
			token_hit_str, ok2 := token_hint.(string)
			if ok2 {
				params["id_token_hint"] = token_hit_str
			}
		}

		// TODO login_hint=email@example.com ?
	}

	// Host ID
	params["ext_agent_host_id"] = getAndPersistHostId(settings)

	// Response Type
	params["response_type"] = "code"

	// Redirect URI
	params["redirect_uri"] = callback_uri

	// Scope - offline_access resource.invoke chatgpt.tokens.use.direct
	params["scope"] = "chatgpt.tokens.use.direct"

	// Resource
	params["resource"] = "https://api.openai.com/v1"

	// State
	params["state"] = rand.Text()

	// Nonce
	params["nonce"] = rand.Text()

	// Code Challenge Method
	params["code_challenge_method"] = "S256"

	// PKCE verifier and the base64url-encoded SHA-256 digest of PKCE verifier without padding
	pkce_verifier = make([]byte, 32)
	rand.Read(pkce_verifier)
	sha_bytes := sha256.Sum256(pkce_verifier)

	params["code_challenge"] = base64.RawURLEncoding.EncodeToString(sha_bytes[:]) // Raw URL encoding is w/o padding

	return
}

func getAndPersistHostId(settings map[string]any) string {
	host, ok := settings["host-id"]
	if ok {
		host_str, ok2 := host.(string)
		if ok2 {
			return host_str
		}
	}

	// gen new id, new flow or if persistance failed
	new_host_id := fmt.Sprintf("urn:uuid:%s", uuid.NewV4().String())

	// save settings after setting host-id
	settings["host-id"] = new_host_id
	core.SaveSettings(settings)

	return new_host_id
}
