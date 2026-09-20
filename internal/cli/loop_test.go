package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSavedRoomLoopAndTransfer(t *testing.T) {
	var server *httptest.Server
	note := ""
	mints := 0
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ons/new" {
			mints++
			fmt.Fprintf(w, `{"url":%q,"expiresAt":"2026-10-11T00:00:00Z"}`, server.URL+"/t/fake-secret/mcp")
			return
		}
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name      string            `json:"name"`
				Arguments map[string]string `json:"arguments"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		result := any(map[string]any{"protocolVersion": "2025-06-18"})
		switch req.Method {
		case "tools/list":
			result = map[string]any{"tools": []map[string]string{{"name": "handoff_post"}, {"name": "handoff_read"}}}
		case "tools/call":
			if req.Params.Name == "handoff_post" {
				note = req.Params.Arguments["text"]
			}
			result = map[string]any{"structuredContent": map[string]string{"text": note}, "content": []map[string]string{{"type": "text", "text": note}}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	defer server.Close()
	c := newClient()
	c.Base = server.URL
	c.Home = filepath.Join(t.TempDir(), "one")
	invoke := func(client Client, input string, args ...string) (int, string) {
		t.Helper()
		var out, stderr bytes.Buffer
		code := run(client, args, strings.NewReader(input), &out, &stderr)
		return code, out.String() + stderr.String()
	}
	if code, msg := invoke(c, "", "create", "demo", "--room", "--json"); code != 0 || strings.Contains(msg, "fake-secret") {
		t.Fatal(code, msg)
	}
	info, err := os.Stat(filepath.Join(c.Home, "demo.grant.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("credential file permissions", err)
	}
	if code, _ := invoke(c, "", "create", "demo", "--room"); code != 1 || mints != 1 {
		t.Fatal("duplicate creation minted another room")
	}
	if code, msg := invoke(c, "continue from checkpoint", "handoff", "demo", "--json"); code != 0 {
		t.Fatal(msg)
	}
	code, grant := invoke(c, "", "export", "demo", "--json")
	if code != 0 {
		t.Fatal(grant)
	}
	other := newClient()
	other.Home = filepath.Join(t.TempDir(), "two")
	if code, msg := invoke(other, grant, "import", "received", "--json"); code != 0 {
		t.Fatal(msg)
	}
	if code, msg := invoke(other, "", "resume", "received", "--json"); code != 0 || !strings.Contains(msg, "continue from checkpoint") {
		t.Fatal(code, msg)
	}
	if code, msg := invoke(other, "", "list", "--json"); code != 0 || strings.Contains(msg, "fake-secret") {
		t.Fatal(code, msg)
	}
	if code, msg := invoke(c, "do not store", "remember", "demo", "--json"); code != 1 {
		t.Fatal("called unavailable tool", msg)
	}
}

func TestRoomFullOffersPurchaseWithoutPromisingMigration(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		fmt.Fprint(w, `{"code":"ROOM_FULL","error":"secret"}`)
	}))
	defer s.Close()
	_, err := newClient().post(s.URL, nil)
	if err == nil || !strings.Contains(err.Error(), "lm world") || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if run(newClient(), []string{"world", "--json"}, nil, &out, &stderr) != 0 || !strings.Contains(out.String(), `"roomMigration":false`) {
		t.Fatal(out.String(), stderr.String())
	}
}

func TestStorageRefusesSymlinksAndInsecurePermissions(t *testing.T) {
	c := newClient()
	c.Home = filepath.Join(t.TempDir(), "private")
	path, err := c.reserve("demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.load("demo"); err == nil {
		t.Fatal("pending grant loaded")
	}
	if _, err = c.reserve("../escape"); err == nil {
		t.Fatal("traversal accepted")
	}
	if err = os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = c.load("demo"); err == nil {
		t.Fatal("insecure grant accepted")
	}
	link := filepath.Join(c.Home, "linked.grant.json")
	if err = os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err = c.load("linked"); err == nil {
		t.Fatal("symlink accepted")
	}
}
