package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const accountIssuer = "https://exciting-kayak-4871.customers.stytch.com"
const accountAuthorize = "https://viibe.to/living-memory/oauth/authorize"
const accountResource = "https://lme.viibe.to/mcp"
const accountSpaces = "https://lme.viibe.to/spaces"

type account struct {
	ClientID string    `json:"clientId"`
	Redirect string    `json:"redirectUri"`
	Access   string    `json:"accessToken,omitempty"`
	Refresh  string    `json:"refreshToken,omitempty"`
	Expires  time.Time `json:"expiresAt"`
}

// Separate from *.grant.json: Room listing/export never sees OAuth credentials.
func (c Client) accountPath() (string, error) {
	if _, err := c.storePath("check"); err != nil {
		return "", err
	}
	return filepath.Join(c.Home, "account.json"), nil
}
func (c Client) lockAccount() (func(), error) {
	p, err := c.accountPath()
	if err != nil {
		return nil, err
	}
	if err = os.Mkdir(p+".lock", 0700); err != nil {
		return nil, errors.New("account is locked; finish the other account command first. After a crashed command, remove account.json.lock only when no account command is running")
	}
	return func() { _ = os.Remove(p + ".lock") }, nil
}
func (c Client) readAccount() (account, error) {
	var a account
	p, err := c.accountPath()
	if err != nil {
		return a, err
	}
	st, err := os.Lstat(p)
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > maxResponse {
		return a, errors.New("private account unavailable; run lm login")
	}
	b, err := os.ReadFile(p)
	if err != nil || json.Unmarshal(b, &a) != nil || a.ClientID == "" {
		return account{}, errors.New("invalid account storage; run lm login")
	}
	return a, nil
}
func (c Client) saveAccount(a account) error {
	p, err := c.accountPath()
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(c.Home, ".account-*")
	if err != nil {
		return errors.New("cannot save private account")
	}
	defer os.Remove(f.Name())
	err = json.NewEncoder(f).Encode(a)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return errors.New("cannot persist private account")
	}
	if os.Rename(f.Name(), p) != nil {
		return errors.New("cannot finalize private account")
	}
	return nil
}
func (c Client) authRequest(endpoint, contentType string, body []byte, result any) error {
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return errors.New("invalid OAuth endpoint")
	}
	req.Header.Set("Content-Type", contentType)
	// Never forward authorization codes or refresh tokens through redirects.
	client := *c.HTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		return errors.New("OAuth request failed; no automatic retry")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("OAuth endpoint answered HTTP %d; run lm login if authorization expired", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if err != nil || len(b) > maxResponse || json.Unmarshal(b, result) != nil {
		return errors.New("invalid OAuth response")
	}
	return nil
}
func (c Client) exchange(a *account, values url.Values) error {
	values.Set("client_id", a.ClientID)
	var t struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Type    string `json:"token_type"`
		Expires int64  `json:"expires_in"`
	}
	if err := c.authRequest(accountIssuer+"/v1/oauth2/token", "application/x-www-form-urlencoded", []byte(values.Encode()), &t); err != nil {
		return err
	}
	if t.Access == "" || strings.ContainsAny(t.Access, "\r\n") || !strings.EqualFold(t.Type, "Bearer") || t.Expires <= 0 || t.Expires > 31536000 {
		return errors.New("invalid OAuth token response")
	}
	a.Access = t.Access
	a.Expires = time.Now().Add(time.Duration(t.Expires) * time.Second)
	if t.Refresh != "" {
		a.Refresh = t.Refresh
	}
	return nil
}

// Each account command holds the account lock, preventing refresh-token rotation races.
type accountTransport struct {
	client  Client
	account *account
	base    http.RoundTripper
}

func (t *accountTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.String() != accountResource && req.URL.String() != accountSpaces {
		return nil, errors.New("account transport refuses non-account destination")
	}
	if t.account.Access == "" || time.Until(t.account.Expires) < 30*time.Second {
		if t.account.Refresh == "" {
			return nil, errors.New("account session expired; run lm login")
		}
		if err := t.client.exchange(t.account, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.account.Refresh}}); err != nil {
			return nil, err
		}
		if err := t.client.saveAccount(*t.account); err != nil {
			return nil, err
		}
	}
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+t.account.Access)
	return t.base.RoundTrip(clone)
}
func (c Client) withAccount(a *account) Client {
	original := c
	client := *c.HTTP
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	client.Transport = &accountTransport{client: original, account: a, base: base}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c.HTTP = &client
	return c
}
func randomOAuth() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", errors.New("cannot generate OAuth randomness")
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func callbackCode(u *url.URL, state string) (string, error) {
	q := u.Query()
	if u.Path != "/callback" || len(q["state"]) != 1 || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1 || len(q["iss"]) != 1 || q.Get("iss") != accountIssuer {
		return "", errors.New("invalid callback state or issuer")
	}
	if q.Get("error") != "" {
		return "", errors.New("authorization denied; run lm login to try again")
	}
	if len(q["code"]) != 1 || q.Get("code") == "" {
		return "", errors.New("authorization code missing")
	}
	return q.Get("code"), nil
}
func openLogin(raw string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", raw)
	case "linux":
		cmd = exec.Command("xdg-open", raw)
	default:
		return
	}
	_ = cmd.Run()
}
func (c Client) login(out, stderr io.Writer, asJSON bool) error {
	unlock, err := c.lockAccount()
	if err != nil {
		return err
	}
	defer unlock()
	a := account{}
	bind := "127.0.0.1:0"
	if old, e := c.readAccount(); e == nil {
		u, e := url.Parse(old.Redirect)
		if e != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Path != "/callback" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			return errors.New("invalid saved OAuth callback")
		}
		a.ClientID = old.ClientID
		a.Redirect = old.Redirect
		bind = u.Host
	}
	listener, err := net.Listen("tcp4", bind)
	if err != nil {
		return errors.New("cannot bind OAuth loopback callback; close the process using its port")
	}
	defer listener.Close()
	if a.ClientID == "" {
		a.Redirect = "http://" + listener.Addr().String() + "/callback"
		data, _ := json.Marshal(map[string]any{"client_name": "Living Memory CLI", "redirect_uris": []string{a.Redirect}, "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}, "token_endpoint_auth_method": "none"})
		var registration struct {
			ID        string   `json:"client_id"`
			Method    string   `json:"token_endpoint_auth_method"`
			Redirects []string `json:"redirect_uris"`
		}
		if err = c.authRequest(accountIssuer+"/v1/oauth2/register", "application/json", data, &registration); err != nil {
			return err
		}
		if registration.ID == "" || registration.Method != "none" || len(registration.Redirects) != 1 || registration.Redirects[0] != a.Redirect {
			return errors.New("invalid public client registration")
		}
		a.ClientID = registration.ID
		if err = c.saveAccount(a); err != nil {
			return err
		}
	}
	state, err := randomOAuth()
	if err != nil {
		return err
	}
	verifier, err := randomOAuth()
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	type received struct {
		code string
		err  error
	}
	done := make(chan received, 1)
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		code, e := callbackCode(r.URL, state)
		if r.Method != "GET" || r.Host != listener.Addr().String() {
			http.Error(w, "Invalid callback.", 400)
			return
		}
		if e != nil && (r.URL.Query().Get("state") != state || r.URL.Query().Get("iss") != accountIssuer || r.URL.Path != "/callback") {
			http.Error(w, "Invalid callback.", 400)
			return
		}
		select {
		case done <- received{code, e}:
			fmt.Fprintln(w, "Returned to lm. You can close this tab.")
		default:
			http.Error(w, "Callback already received.", 409)
		}
	})}
	go server.Serve(listener)
	defer server.Close()
	q := url.Values{"client_id": {a.ClientID}, "redirect_uri": {a.Redirect}, "response_type": {"code"}, "scope": {"openid offline_access"}, "state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}, "resource": {accountResource}}
	link := accountAuthorize + "?" + q.Encode()
	fmt.Fprintln(stderr, "Open this URL on this computer and approve within 5 minutes:\n"+link)
	opener := c.openBrowser
	if opener == nil {
		opener = openLogin
	}
	go opener(link)
	ctx, cancel := context.WithTimeout(context.Background(), loginWait(c.loginTimeout))
	defer cancel()
	select {
	case response := <-done:
		if response.err != nil {
			return response.err
		}
		if err = c.exchange(&a, url.Values{"grant_type": {"authorization_code"}, "code": {response.code}, "redirect_uri": {a.Redirect}, "code_verifier": {verifier}, "resource": {accountResource}}); err != nil {
			return err
		}
	case <-ctx.Done():
		return errors.New("login timed out; run lm login again")
	}
	if err = c.saveAccount(a); err != nil {
		return err
	}
	if asJSON {
		return json.NewEncoder(out).Encode(map[string]any{"signedIn": true, "next": "lm list"})
	}
	_, err = fmt.Fprintln(out, "Signed in. Next: lm list")
	return err
}

func loginWait(d time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return 5 * time.Minute
}
