package cli

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type authRoundTrip func(*http.Request) (*http.Response, error)

func (f authRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func authResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func TestCallbackValidation(t *testing.T) {
	good := url.Values{"state": {"expected"}, "iss": {accountIssuer}, "code": {"secret-code"}}
	for _, tc := range []struct {
		name   string
		mutate func(url.Values)
		ok     bool
	}{
		{"valid", func(q url.Values) {}, true},
		{"wrong state", func(q url.Values) { q.Set("state", "wrong") }, false},
		{"missing issuer", func(q url.Values) { q.Del("iss") }, false},
		{"legacy issuer", func(q url.Values) { q.Set("iss", "stytch.com/project-live-other") }, false},
		{"duplicate state", func(q url.Values) { q.Add("state", "expected") }, false},
		{"duplicate code", func(q url.Values) { q.Add("code", "second") }, false},
		{"denied", func(q url.Values) { q.Set("error", "access_denied") }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, _ := url.ParseQuery(good.Encode())
			tc.mutate(q)
			_, err := callbackCode(&url.URL{Path: "/callback", RawQuery: q.Encode()}, "expected")
			if (err == nil) != tc.ok {
				t.Fatal(err)
			}
		})
	}
}
func TestAccountRefreshRotatesBeforeMCPAndPersists(t *testing.T) {
	c := newClient()
	c.Home = filepath.Join(t.TempDir(), "private")
	a := account{ClientID: "client", Access: "expired", Refresh: "old-refresh", Expires: time.Now().Add(-time.Hour)}
	if err := c.saveAccount(a); err != nil {
		t.Fatal(err)
	}
	refreshes, calls := 0, 0
	c.HTTP = &http.Client{Transport: authRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() == accountIssuer+"/v1/oauth2/token" {
			refreshes++
			b, _ := io.ReadAll(r.Body)
			q, _ := url.ParseQuery(string(b))
			if q.Get("refresh_token") != "old-refresh" || q.Get("client_id") != "client" {
				t.Fatal("invalid refresh request")
			}
			return authResponse(200, `{"access_token":"fresh","refresh_token":"rotated","token_type":"Bearer","expires_in":3600}`), nil
		}
		calls++
		if r.URL.String() != accountResource || r.Header.Get("Authorization") != "Bearer fresh" {
			t.Fatal("wrong credential destination")
		}
		saved, err := c.readAccount()
		if err != nil || saved.Refresh != "rotated" {
			t.Fatal("must persist rotation before operation")
		}
		return authResponse(200, `{}`), nil
	})}
	connected := c.withAccount(&a)
	for i := 0; i < 2; i++ {
		if _, err := connected.post(accountResource, []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	if refreshes != 1 || calls != 2 {
		t.Fatal(refreshes, calls)
	}
	if _, err := connected.post("https://other.example/mcp", nil); err == nil {
		t.Fatal("forwarded token to stranger")
	}
	if calls != 2 {
		t.Fatal("unexpected transport call")
	}
}
func TestFailedRefreshDoesNotCallMCP(t *testing.T) {
	c := newClient()
	c.Home = filepath.Join(t.TempDir(), "private")
	c.HTTP = &http.Client{Transport: authRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != accountIssuer+"/v1/oauth2/token" {
			t.Fatal("MCP called after refresh failure")
		}
		return authResponse(400, `{"error":"invalid_grant","detail":"secret-token"}`), nil
	})}
	a := account{ClientID: "client", Refresh: "secret-token"}
	_, err := c.withAccount(&a).post(accountResource, nil)
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatal(err)
	}
}
func TestAccountPrivateStorageAndRoomIsolation(t *testing.T) {
	c := newClient()
	c.Home = filepath.Join(t.TempDir(), "private")
	a := account{ClientID: "client", Access: "private-access", Refresh: "private-refresh"}
	if err := c.saveAccount(a); err != nil {
		t.Fatal(err)
	}
	path, _ := c.accountPath()
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
	unlock, err := c.lockAccount()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.lockAccount(); err == nil {
		t.Fatal("concurrent account lock acquired")
	}
	unlock()
	for _, args := range [][]string{{"list", "--json"}, {"export", "account", "--json"}, {"export", "--account", "--json"}} {
		_, out, stderr := invokeTest(c, "", args...)
		if strings.Contains(out+stderr, "private-access") || strings.Contains(out+stderr, "private-refresh") {
			t.Fatal("leaked account")
		}
	}
	os.Chmod(path, 0644)
	if _, err = c.readAccount(); err == nil {
		t.Fatal("accepted public account file")
	}
	os.Chmod(path, 0600)
	code, _, stderr := invokeTest(c, "", "logout")
	if code != 0 {
		t.Fatal(stderr)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("logout retained credential")
	}
}

func TestClientRegistrationWithoutTokensIsSignedOut(t *testing.T) {
	c := newClient()
	c.Home = filepath.Join(t.TempDir(), "private")
	c.HTTP = &http.Client{Transport: rejectRequests{t}}
	registration := account{ClientID: "registered-client", Redirect: "http://127.0.0.1:61292/callback"}
	if err := c.saveAccount(registration); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := invokeTest(c, "", "list", "--json")
	if code != 0 || stderr != "" || !strings.Contains(out, `"remoteStatus":"signed_out"`) || strings.Contains(out, "registered-client") {
		t.Fatal(code, out, stderr)
	}
	code, out, stderr = invokeTest(c, "", "world", "--json")
	if code != 0 || stderr != "" || !strings.Contains(out, `"status":"signed_out"`) || !strings.Contains(out, `"next":"lm login"`) {
		t.Fatal(code, out, stderr)
	}
	code, out, stderr = invokeTest(c, "", "inspect", "--account")
	if code == 0 || out != "" || !strings.Contains(stderr, "run lm login") {
		t.Fatal(code, out, stderr)
	}
	saved, err := c.readAccount()
	if err != nil || saved.ClientID != registration.ClientID || saved.Redirect != registration.Redirect {
		t.Fatal("registration was lost instead of retained for login", err)
	}
}
func TestOAuthRejectsRedirect(t *testing.T) {
	c := newClient()
	requests := 0
	c.HTTP = &http.Client{Transport: authRoundTrip(func(r *http.Request) (*http.Response, error) {
		requests++
		res := authResponse(307, "")
		res.Header.Set("Location", "https://other.example/steal")
		return res, nil
	})}
	var output map[string]any
	if err := c.authRequest(accountIssuer+"/v1/oauth2/token", "application/json", []byte(`{}`), &output); err == nil {
		t.Fatal("accepted redirect")
	}
	if requests != 1 {
		t.Fatal("followed credential redirect")
	}
}
func TestAccountInspectUsesBearerWithoutExportableGrant(t *testing.T) {
	c := newClient()
	c.Home = filepath.Join(t.TempDir(), "private")
	if err := c.saveAccount(account{ClientID: "client", Access: "secret-access", Expires: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	c.HTTP = &http.Client{Transport: authRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != accountResource || r.Header.Get("Authorization") != "Bearer secret-access" {
			t.Fatal("missing account auth")
		}
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		result := `{"protocolVersion":"2025-06-18"}`
		if req.Method == "tools/list" {
			result = `{"tools":[]}`
		}
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": json.RawMessage(result)})
		return authResponse(200, string(b)), nil
	})}
	code, out, stderr := invokeTest(c, "", "inspect", "--account", "--json")
	if code != 0 || strings.Contains(out+stderr, "secret-access") || !strings.Contains(out, `"kind":"world"`) {
		t.Fatal(code, out, stderr)
	}
	files, _ := filepath.Glob(filepath.Join(c.Home, "*.grant.json"))
	if len(files) != 0 {
		t.Fatal("created Room grant for account")
	}
}

func TestLoginPKCECallbackAndPrivatePersistence(t *testing.T) {
	c := newClient()
	c.Home = filepath.Join(t.TempDir(), "private")
	c.loginTimeout = 3 * time.Second
	challenge := ""
	redirect := ""
	calls := 0
	c.HTTP = &http.Client{Transport: authRoundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.URL.String() {
		case accountIssuer + "/v1/oauth2/register":
			var req struct {
				Redirects []string `json:"redirect_uris"`
				Method    string   `json:"token_endpoint_auth_method"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			if len(req.Redirects) != 1 || req.Method != "none" {
				t.Error("not a public loopback registration")
			}
			redirect = req.Redirects[0]
			b, _ := json.Marshal(map[string]any{"client_id": "test-client", "token_endpoint_auth_method": "none", "redirect_uris": req.Redirects})
			return authResponse(201, string(b)), nil
		case accountIssuer + "/v1/oauth2/token":
			data, _ := io.ReadAll(r.Body)
			q, _ := url.ParseQuery(string(data))
			h := sha256.Sum256([]byte(q.Get("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(h[:]) != challenge || q.Get("redirect_uri") != redirect || q.Get("code") != "test-code" || q.Get("resource") != accountResource {
				t.Error("PKCE exchange mismatch")
			}
			return authResponse(200, `{"access_token":"login-secret","refresh_token":"refresh-secret","token_type":"Bearer","expires_in":3600}`), nil
		case accountResource:
			calls++
			if r.Header.Get("Authorization") != "Bearer login-secret" {
				t.Error("missing bearer")
			}
			var req struct {
				ID     int    `json:"id"`
				Method string `json:"method"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			result := `{"protocolVersion":"2025-06-18"}`
			if req.Method == "tools/list" {
				result = `{"tools":[]}`
			}
			b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": json.RawMessage(result)})
			return authResponse(200, string(b)), nil
		}
		t.Error("unexpected destination")
		return authResponse(500, ""), nil
	})}
	c.openBrowser = func(raw string) {
		u, _ := url.Parse(raw)
		q := u.Query()
		challenge = q.Get("code_challenge")
		if q.Get("code_challenge_method") != "S256" || q.Get("scope") != "openid offline_access" {
			t.Error("invalid authorization request")
		}
		// Wrong state must not consume the one-use callback.
		cb, _ := url.Parse(q.Get("redirect_uri"))
		v := url.Values{"state": {"wrong"}, "iss": {accountIssuer}, "code": {"test-code"}}
		cb.RawQuery = v.Encode()
		res, err := http.Get(cb.String())
		if err != nil {
			t.Error(err)
			return
		}
		res.Body.Close()
		if res.StatusCode != 400 {
			t.Error("wrong state accepted")
		}
		v.Set("state", q.Get("state"))
		cb.RawQuery = v.Encode()
		res, err = http.Get(cb.String())
		if err == nil {
			res.Body.Close()
		}
	}
	code, out, stderr := invokeTest(c, "", "login", "--json")
	if code != 0 || calls != 0 || !strings.Contains(out, `"signedIn":true`) || strings.Contains(out+stderr, "login-secret") || strings.Contains(out+stderr, "refresh-secret") {
		t.Fatal(code, out, stderr)
	}
	a, err := c.readAccount()
	if err != nil || a.Refresh != "refresh-secret" {
		t.Fatal("credentials not persisted", err)
	}
}

func TestLoginTimeoutPreservesExistingAccount(t *testing.T) {
	c := newClient()
	c.Home = filepath.Join(t.TempDir(), "private")
	c.loginTimeout = 20 * time.Millisecond
	a := account{ClientID: "existing", Redirect: "http://127.0.0.1:0/callback", Access: "keep-access", Refresh: "keep-refresh"}
	// A saved real registration has a fixed port; zero is a local test listener only.
	if err := c.saveAccount(a); err != nil {
		t.Fatal(err)
	}
	c.openBrowser = func(string) {}
	code, _, _ := invokeTest(c, "", "login")
	if code == 0 {
		t.Fatal("timeout accepted")
	}
	after, err := c.readAccount()
	if err != nil || after.Access != a.Access || after.Refresh != a.Refresh {
		t.Fatal("lost previous account")
	}
	unlock, err := c.lockAccount()
	if err != nil {
		t.Fatal("timeout retained lock")
	}
	unlock()
}
