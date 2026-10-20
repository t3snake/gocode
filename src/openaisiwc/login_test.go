package openaisiwc

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/t3snake/gocode/src/core"
)

// Only the remote endpoints are redirected. Token exchange, callback HTTP, and
// settings storage all run through the real code.
type localAuthTransport struct {
	server_url *url.URL
	transport  http.RoundTripper
}

func (transport localAuthTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Host != "auth.openai.com" {
		return nil, fmt.Errorf("unexpected remote host: %s", request.URL.Host)
	}
	request = request.Clone(request.Context())
	request.URL.Scheme = transport.server_url.Scheme
	request.URL.Host = transport.server_url.Host
	return transport.transport.RoundTrip(request)
}

type loginProvider struct {
	mutex     sync.Mutex
	t         *testing.T
	key       *rsa.PrivateKey
	params    url.Values
	outcome   string
	id_token  string
	exchanges int
}

func (provider *loginProvider) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	provider.mutex.Lock()
	defer provider.mutex.Unlock()
	writer.Header().Set("Content-Type", "application/json")
	if request.URL.Path == "/.well-known/jwks.json" {
		json.MarshalWrite(writer, struct {
			Keys []signingKey `json:"keys"`
		}{Keys: []signingKey{publicSigningKey(provider.key)}})
		return
	}
	if request.URL.Path != "/api/accounts/oauth/token" || request.Method != http.MethodPost {
		provider.t.Errorf("unexpected request: %s %s", request.Method, request.URL)
		http.Error(writer, "unexpected request", http.StatusNotFound)
		return
	}
	provider.exchanges++
	if err := request.ParseForm(); err != nil {
		provider.t.Error(err)
		http.Error(writer, "invalid form", http.StatusBadRequest)
		return
	}
	form := request.PostForm
	challenge := sha256.Sum256([]byte(form.Get("code_verifier")))
	if form.Get("grant_type") != "authorization_code" || form.Get("client_id") != "oaiapp_test" ||
		form.Get("code") != "test-code" || form.Get("redirect_uri") != provider.params.Get("redirect_uri") ||
		form.Get("resource") != "https://api.openai.com/v1" ||
		base64.RawURLEncoding.EncodeToString(challenge[:]) != provider.params.Get("code_challenge") {
		provider.t.Error("token exchange did not match the authorization request")
		http.Error(writer, "invalid grant", http.StatusBadRequest)
		return
	}
	if provider.outcome == "exchange failure" {
		writer.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(writer, `{"error":"invalid_grant"}`)
		return
	}
	subject := "user"
	if provider.outcome == "different account" {
		subject = "another-user"
	}
	claims := fmt.Sprintf(`{"iss":"https://auth.openai.com","sub":%q,"aud":"oaiapp_test","nonce":%q,"iat":%d,"exp":%d}`,
		subject, provider.params.Get("nonce"), time.Now().Unix(), time.Now().Add(time.Hour).Unix())
	token, err := signedToken(provider.key, `{"alg":"RS256","kid":"test"}`, claims)
	if err != nil {
		provider.t.Error(err)
		http.Error(writer, "cannot sign token", http.StatusInternalServerError)
		return
	}
	if provider.outcome == "bad signature" {
		parts := strings.Split(token, ".")
		parts[2] = base64.RawURLEncoding.EncodeToString(make([]byte, 256))
		token = strings.Join(parts, ".")
	}
	provider.id_token = token
	scope := "openid chatgpt.tokens.use.direct"
	if provider.outcome == "no plan scope" {
		scope = "openid chatgpt.tokens.use.direct.other"
	}
	json.MarshalWrite(writer, tokenResponse{
		AccessToken: "new-access", RefreshToken: "new-refresh", IDToken: token,
		TokenType: "Bearer", ExpiresIn: 3600, Scope: scope,
	})
}

func beginLogin(t *testing.T, ctx context.Context) (url.Values, <-chan error) {
	t.Helper()
	reader, writer := io.Pipe()
	t.Cleanup(func() { reader.Close() })
	done := make(chan error, 1)
	go func() {
		err := LoginChatGPT(ctx, writer)
		writer.CloseWithError(err)
		done <- err
		close(done)
	}()
	scanner := bufio.NewScanner(reader)
	if !scanner.Scan() {
		t.Fatalf("sign-in link was not printed: %v", scanner.Err())
	}
	link := scanner.Text()
	target, visible, found := strings.Cut(strings.TrimPrefix(link, "\x1b]8;;"), "\x1b\\")
	if !found || !strings.HasPrefix(link, "\x1b]8;;") || visible != "\x1b[4;36mContinue with ChatGPT\x1b[0m\x1b]8;;\x1b\\" {
		t.Fatalf("expected a styled OSC 8 sign-in label: %q", link)
	}
	scanner.Scan() // Blank line.
	scanner.Scan() // Copy-and-paste instruction.
	if !scanner.Scan() || scanner.Text() != target {
		t.Fatal("plain URL does not match the hyperlink target")
	}
	uri, err := url.Parse(target)
	if err != nil || uri.Scheme != "https" || uri.Host != "auth.openai.com" || uri.Path != "/api/accounts/authorize" {
		t.Fatalf("invalid sign-in URL: %v", err)
	}
	return uri.Query(), done
}

func callbackStatus(t *testing.T, params, query url.Values) int {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(params.Get("redirect_uri") + "?" + query.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	io.Copy(io.Discard, response.Body)
	return response.StatusCode
}

func TestLoginFlow(t *testing.T) {
	config_dir := t.TempDir()
	t.Setenv("HOME", config_dir)
	t.Setenv("XDG_CONFIG_HOME", config_dir)
	t.Setenv("AppData", config_dir)
	if err := core.LoadSettings(); err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	provider := &loginProvider{t: t, key: key}
	server := httptest.NewServer(provider)
	defer server.Close()
	server_url, _ := url.Parse(server.URL)
	previous_client := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: localAuthTransport{server_url, server.Client().Transport}}
	defer func() { http.DefaultClient = previous_client }()

	var previous_params url.Values
	var host_id string
	test_cases := []struct {
		name        string
		want_client string
		want_error  string
	}{
		{"exchange failure", "dynamic_agent_client", "invalid_grant"},
		{"retry", "oaiapp_test", ""},
		{"returning account", "oaiapp_test", ""},
		{"no plan scope", "oaiapp_test", ""},
		{"different account", "oaiapp_test", "does not match the saved account"},
		{"bad signature", "oaiapp_test", "signature"},
		{"wrong callback client", "oaiapp_test", "client ID does not match"},
		{"denied", "oaiapp_test", "access_denied"},
		{"cancel", "oaiapp_test", "context canceled"},
	}
	for _, tt := range test_cases {
		t.Run(tt.name, func(t *testing.T) {
			before, err := core.GetSettings()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			params, done := beginLogin(t, ctx)
			defer func() {
				cancel()
				for range done {
				}
			}()
			if params.Get("client_id") != tt.want_client || params.Get("id_token_hint") != before.IDTokenHint {
				t.Fatal("sign-in did not use the saved registration")
			}
			if host_id == "" {
				host_id = params.Get("ext_agent_host_id")
			}
			if !strings.HasPrefix(host_id, "urn:uuid:") || params.Get("ext_agent_host_id") != host_id {
				t.Fatal("host ID missing or changed")
			}
			if params.Get("state") == "" || params.Get("nonce") == "" || params.Get("code_challenge_method") != "S256" {
				t.Fatal("missing sign-in protection")
			}
			if previous_params != nil && (params.Get("state") == previous_params.Get("state") ||
				params.Get("nonce") == previous_params.Get("nonce") || params.Get("code_challenge") == previous_params.Get("code_challenge")) {
				t.Fatal("sign-in reused temporary secrets")
			}
			previous_params = params
			provider.mutex.Lock()
			provider.params = params
			provider.outcome = tt.name
			provider.exchanges = 0
			provider.mutex.Unlock()
			query := url.Values{"state": {params.Get("state")}, "code": {"test-code"}}
			if tt.want_client == "dynamic_agent_client" {
				query.Set("client_id", "oaiapp_test")
			}
			if tt.name == "cancel" {
				cancel()
			} else {
				// A wrong state must leave the real sign-in attempt alive.
				query.Set("state", "wrong-state")
				if callbackStatus(t, params, query) != http.StatusBadRequest {
					t.Fatal("wrong state accepted")
				}
				query.Set("state", params.Get("state"))
				want_status := http.StatusOK
				if tt.name == "denied" {
					query.Set("error", "access_denied")
					want_status = http.StatusBadRequest
				}
				if tt.name == "wrong callback client" {
					query.Set("client_id", "other")
					want_status = http.StatusBadRequest
				}
				if status := callbackStatus(t, params, query); status != want_status {
					t.Fatalf("callback status = %d, want %d", status, want_status)
				}
			}
			err = <-done
			if tt.want_error != "" {
				if err == nil || !strings.Contains(err.Error(), tt.want_error) {
					t.Fatalf("got %v, want error containing %q", err, tt.want_error)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			want_exchanges := 1
			if tt.name == "cancel" || tt.name == "denied" || tt.name == "wrong callback client" {
				want_exchanges = 0
			}
			provider.mutex.Lock()
			exchanges := provider.exchanges
			id_token := provider.id_token
			provider.mutex.Unlock()
			if exchanges != want_exchanges {
				t.Fatalf("exchanged %d codes, want %d", exchanges, want_exchanges)
			}
			if err := core.LoadSettings(); err != nil {
				t.Fatal(err)
			}
			saved, err := core.GetSettings()
			if err != nil {
				t.Fatal(err)
			}
			if tt.name == "exchange failure" {
				if saved != (core.Settings{HostID: host_id, PendingOpenAIClientID: "oaiapp_test"}) {
					t.Fatal("failed registration was not saved for retry")
				}
			} else if tt.want_error != "" {
				if saved != before {
					t.Fatal("failed sign-in changed the saved account")
				}
			} else if saved.OpenAIClientID != "oaiapp_test" || saved.PendingOpenAIClientID != "" || saved.Subject != "user" ||
				saved.IDTokenHint != id_token || saved.AccessToken != "new-access" || saved.RefreshToken != "new-refresh" ||
				saved.ExpiresAt.Before(time.Now().Add(59*time.Minute)) || PlanUsageEnabled(saved.Scope) != (tt.name != "no plan scope") {
				t.Fatal("verified credentials were not saved")
			}
			config_path, err := core.GetOrInitConfig()
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(config_path)
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
				t.Fatalf("credentials file has permissions %o", info.Mode().Perm())
			}
			client := &http.Client{Timeout: time.Second}
			response, err := client.Get(params.Get("redirect_uri"))
			if err == nil {
				response.Body.Close()
				t.Fatal("callback listener still running after sign-in ended")
			}
		})
	}
}

func TestCallbackUsedOnce(t *testing.T) {
	listener, err := startCallbackListener("state", "oaiapp_test")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.server.Close()
	params := url.Values{"redirect_uri": {listener.uri}}
	query := url.Values{"state": {"state"}, "code": {"test-code"}}
	client := &http.Client{Timeout: time.Second}
	response, err := client.Head(listener.uri + "?" + query.Encode())
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Fatal("HEAD consumed the sign-in callback")
	}
	if callbackStatus(t, params, query) != http.StatusOK {
		t.Fatal("first callback rejected")
	}
	select {
	case result := <-listener.results:
		if result.err != nil {
			t.Fatal(result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("first callback was not delivered")
	}
	if callbackStatus(t, params, query) != http.StatusConflict {
		t.Fatal("repeated callback accepted after the first result was read")
	}
	select {
	case <-listener.results:
		t.Fatal("repeated callback delivered a second result")
	default:
	}
}
