# Sign in with ChatGPT for gocode

Letting a user grant access without sharing their password is called **OAuth**. Verifying who signed in is provided by **OpenID Connect (OIDC)**. Sign in with ChatGPT (**SIWC**) uses both.

The browser returns to a small HTTP listener in gocode; this is the **callback**. A temporary code returned there is the **authorization code**, which gocode exchanges for credentials.

## Values used in the diagram

| Purpose | Name | Created by |
| --- | --- | --- |
| Stable identifier for this machine | `ext_agent_host_id` | gocode, once per host |
| Registration for the authorized account and workspace | `client_id` | OpenAI during first registration |
| Random value connecting the callback to the pending login | `state` | gocode, fresh per login |
| Random value connecting the signed identity document to the pending login | nonce | gocode, fresh per login |
| Proof that the app exchanging the code started the login | **PKCE** (Proof Key for Code Exchange) | gocode creates a secret verifier and its challenge |
| Fingerprint of the secret verifier | PKCE challenge | gocode: URL-safe encoding of SHA-256(verifier), without padding |
| Permissions requested or granted | scopes | gocode requests; OpenAI returns approved scopes |
| Credential for model requests | access token | OpenAI |
| Credential for renewing access | refresh token | OpenAI |
| Signed document identifying the user | ID token | OpenAI |

## Sequence

```mermaid
sequenceDiagram
    autonumber
    actor User
    participant G as gocode
    participant S as Protected local storage
    participant B as System browser
    participant A as OpenAI authorization
    participant T as OpenAI token endpoint
    participant R as OpenAI Responses API

    User->>G: Continue with ChatGPT
    G->>S: Load or create stable ext_agent_host_id
    S-->>G: Host ID and selected registration, if saved
    G->>G: Start listener on 127.0.0.1 at /auth/callback
    G->>G: Generate fresh state, nonce, verifier
    G->>G: Derive PKCE challenge from verifier

    alt First registration
        G->>B: Open login with dynamic_agent_client and agent_name_hint=gocode
    else Existing registration
        G->>B: Open login with saved issued client_id
    end
    Note over G,B: Also send host ID, callback URI, scopes, resource,<br/>state, nonce, challenge, and challenge method S256.<br/>Keep the verifier in gocode memory.
    B->>A: Request sign-in and authorization
    User->>A: Sign in and approve requested access
    A-->>B: Redirect to callback with code and state<br/>Also return issued client_id for first registration
    B->>G: Deliver callback to local listener
    G->>G: Check state, timeout, one-time use, and errors
    Note over G: Reject invalid callbacks.<br/>Require issued client_id for a new registration.<br/>For returning login, reject a different client_id.

    G->>T: Exchange code with issued client_id, verifier,<br/>same callback URI, and resource
    T->>T: Validate code and check verifier against challenge
    T-->>G: Access token, refresh token, ID token,<br/>granted scopes and expiry information
    G->>G: Verify ID token signature, issuer, audience,<br/>expiry, subject, and nonce
    Note over G: For returning login, require the verified identity<br/>to match the selected saved account.
    G->>S: Save verified account, issued client_id,<br/>tokens, granted scopes, and expiry securely
    G->>G: Close listener and discard temporary login values

    alt Plan usage permission granted
        G->>R: POST /v1/responses with bearer access token<br/>store=false, stream=true, model and input history
        R-->>G: Stream output and terminal event
        G-->>User: Display output, success requires response.completed
    else Plan usage permission missing
        G-->>User: Signed in, ChatGPT plan usage disabled
    end

    opt Access token needs renewal
        G->>S: Load selected registration and refresh token
        G->>G: Lock refresh for this session
        G->>T: Refresh using issued client_id,<br/>refresh_token and resource
        T-->>G: Replacement access and refresh tokens<br/>with expiry information
        G->>S: Save replacement credentials together atomically
        G->>G: Unlock refresh, use replacement access token
        Note over G,T: If the session is expired or revoked, sign in again.<br/>Keep network failures separate from rejected credentials.
    end
```

## What to keep

- **Persist:** stable host ID; each registration's issued client ID and verified issuer/subject; tokens; granted scopes; expiry information. Email or an account label helps the user choose a registration.
- **Memory only:** state, nonce, PKCE verifier/challenge, callback URI for the attempt, and authorization code. Discard after success, failure, or timeout.
- **Secret:** access and refresh tokens, and the PKCE verifier. Treat the ID token as sensitive too. Store tokens in protected storage; never put them in `sessionLog.txt`, source control, prompts, or tool output.
- **Not secret credentials:** host ID, client ID, scopes, expiry, and PKCE challenge. State and nonce travel through the browser but must be unpredictable. Protect stored account mappings from accidental replacement.

## Required checks and configuration

- Request identity scopes `openid profile email` and plan scopes `offline_access resource.invoke chatgpt.tokens.use.direct`. Check the returned scopes for `chatgpt.tokens.use.direct` before model requests.
- Use `http://127.0.0.1:<port>/auth/callback`, not `localhost`. Keep the exact URI during an attempt; only the port may vary between attempts.
- The ID token's fields are called **claims**: `iss` identifies the issuer, `aud` the intended client, `exp` the expiry, and `sub` the account. Verify the signature using OpenAI's published public keys, called **JWKS**, through a maintained library before trusting claims or nonce.
- No client secret is required. `dynamic_agent_client` starts registration; token exchange and refresh use the actual issued client ID.
- Keep registrations separate, even if their emails match. Serialize refreshes, including across processes, because refresh tokens are replaced.
- A protected file is an option: use owner-only permissions (`0600` on Unix) and atomic writes. Retain the ID token for later `id_token_hint` use; redact login URLs containing that hint.
- gocode currently calls Chat Completions. Add a Responses API path and adapt conversation history and tool messages for this SIWC route.

```text
Authorization: https://auth.openai.com/api/accounts/authorize
Token exchange and refresh: https://auth.openai.com/api/accounts/oauth/token
Resource: https://api.openai.com/v1
Model requests: https://api.openai.com/v1/responses
```

Official OpenAI documentation: [registration and sign-in](https://developers.openai.com/siwc/token-sharing-open-source/sign-in), [account storage and refresh](https://developers.openai.com/siwc/token-sharing-open-source/profiles-and-sessions), [Responses requests](https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference).
