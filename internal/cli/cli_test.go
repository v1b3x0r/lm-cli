package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateAndInspect(t *testing.T) {
	for _, sse := range []bool{false, true} {
		t.Run(fmt.Sprint(sse), func(t *testing.T) {
			var server *httptest.Server
			calls := 0
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "POST" {
					t.Errorf("method %s", r.Method)
				}
				if r.URL.Path == "/ons/new" {
					w.WriteHeader(201)
					fmt.Fprintf(w, `{"url":%q,"expiresAt":"2026-10-11T00:00:00.000Z","note":"server lifecycle","roomId":"w_0123456789abcdef0123456789abcdef"}`, server.URL+"/t/fake-credential/mcp")
					return
				}
				var request struct {
					ID     int            `json:"id"`
					Method string         `json:"method"`
					Params map[string]any `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				result := `{"protocolVersion":"2025-06-18"}`
				if request.Method == "tools/list" {
					result = `{"tools":[{"name":"memory_search"}],"nextCursor":"second"}`
					if request.Params["cursor"] == "second" {
						result = `{"tools":[{"name":"handoff_read"}]}`
					}
				}
				payload := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":%s}`, request.ID, result)
				if sse {
					fmt.Fprintf(w, "event: message\ndata: %s\n\n", payload)
				} else {
					fmt.Fprint(w, payload)
				}
			}))
			defer server.Close()
			c := newClient()
			c.Home = t.TempDir() + "/private"
			c.Base = server.URL
			var out, stderr bytes.Buffer
			if code := run(c, []string{"create", "sample", "--room", "--json"}, strings.NewReader(""), &out, &stderr); code != 0 {
				t.Fatal(stderr.String())
			}
			if !strings.Contains(out.String(), "fake-credential") {
				t.Fatal("create must show the MCP address")
			}
			grant, err := c.load("sample")
			if err != nil {
				t.Fatal(err)
			}
			if grant.Name != "sample" || grant.RoomID != testRoomID || grant.Note != "server lifecycle" || grant.Warning == "" {
				t.Fatal("incomplete saved grant")
			}
			out.Reset()
			if run(c, []string{"export", "sample", "--json"}, nil, &out, &stderr) != 0 {
				t.Fatal(stderr.String())
			}
			input := out.String()
			out.Reset()
			if code := run(c, []string{"inspect", "--json"}, strings.NewReader(input), &out, &stderr); code != 0 {
				t.Fatal(stderr.String())
			}
			if !strings.Contains(out.String(), "fake-credential") {
				t.Fatal("inspect must show the MCP address")
			}
			var info Inspection
			if err := json.Unmarshal(out.Bytes(), &info); err != nil {
				t.Fatal(err)
			}
			if len(info.Tools) != 2 || calls != 4 {
				t.Fatalf("tools=%v calls=%d", info.Tools, calls)
			}
		})
	}
}

func TestErrorsDoNotLeakOrRetry(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(429)
		fmt.Fprint(w, "fake-secret")
	}))
	defer server.Close()
	c := newClient()
	c.Home = t.TempDir() + "/private"
	c.Base = server.URL
	var out, stderr bytes.Buffer
	if run(c, []string{"create", "sample", "--room", "--json"}, nil, &out, &stderr) != 1 {
		t.Fatal("expected failure")
	}
	if calls != 1 || out.Len() != 0 || strings.Contains(stderr.String(), "fake-secret") || !strings.Contains(stderr.String(), "429") {
		t.Fatal("unsafe error", stderr.String())
	}
}

func TestInvalidInputDoesNotMakeRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected network request") }))
	defer server.Close()
	c := newClient()
	c.Home = t.TempDir() + "/private"
	c.Base = server.URL
	for _, args := range [][]string{{"create", "x"}, {"create", "x", "--world"}, {"create", "x", "extra", "--room"}, {"inspect", "--room"}, {"delete", "x"}} {
		var out, stderr bytes.Buffer
		if run(c, args, strings.NewReader("{}"), &out, &stderr) != 1 {
			t.Errorf("accepted %v", args)
		}
	}
	for _, address := range []string{"http://example.com/t/secret/mcp", "https://user:secret@example.com/mcp", "https://example.com/mcp?token=secret", "not a url"} {
		if validAddress(address) {
			t.Errorf("accepted %s", address)
		}
	}
}

func TestRedirectNotFollowed(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("followed redirect") }))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	c := newClient()
	c.Home = t.TempDir() + "/private"
	c.Base = origin.URL
	if _, err := c.Create("test"); err == nil {
		t.Fatal("accepted redirect")
	}
}

func TestMCPRejectsInvalidAndErrorResponses(t *testing.T) {
	for _, body := range []string{`{"jsonrpc":"2.0","id":999,"result":{}}`, `{"jsonrpc":"2.0","id":1,"error":{"message":"secret"}}`, `{"jsonrpc":"2.0","id":1,"result":null}`, `not json`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		var result any
		err := newClient().rpc(server.URL, "initialize", nil, 1, &result)
		server.Close()
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unsafe result: %v", err)
		}
	}
}
