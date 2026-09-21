package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

const testRoomID = "w_0123456789abcdef0123456789abcdef"
const testToken = "ons_abcdef0123456789abcdef0123456789"

func TestAddressesRespectDoorAndOrigin(t *testing.T) {
	for _, tc := range []struct{ endpoint, access, fragment string }{
		{"https://lme.viibe.to/t/" + testToken + "/mcp", "read_write", testToken},
		{"https://lme.viibe.to/t/ro_abcdef0123456789abcdef0123456789/mcp/", "read_only", "ro_abcdef0123456789abcdef0123456789"},
		{"https://other.example/t/" + testToken + "/mcp", "unknown", ""},
		{"https://lme.viibe.to/t/" + testRoomID + "/mcp", "unknown", ""},
		{"https://lme.viibe.to/t/ons_bad/mcp", "unknown", ""},
		{"https://lme.viibe.to/t/lmr_other/mcp", "unknown", ""},
	} {
		t.Run(tc.endpoint, func(t *testing.T) {
			s := summary(Grant{Name: "local", RoomID: testRoomID, URL: tc.endpoint})
			a := s.Addresses
			if a.MCP != tc.endpoint || a.Access != tc.access || shown(s.Identity.RoomID) != testRoomID {
				t.Fatal(s)
			}
			if tc.fragment == "" {
				if a.Open != nil || a.Guide != nil {
					t.Fatal("fabricated Theatre address")
				}
			} else {
				expected := "https://living-memory.app/theatre#" + tc.fragment
				if shown(a.Open) != expected || shown(a.Guide) != expected {
					t.Fatal(a)
				}
			}
			if tc.access == "read_only" && strings.Contains(a.Warning, "read and write") {
				t.Fatal("claimed write permission")
			}
			if tc.access == "unknown" && !strings.Contains(a.Warning, "unknown") {
				t.Fatal("guessed access")
			}
		})
	}
}

func saveTestGrant(t *testing.T, c Client, g Grant) {
	t.Helper()
	path, err := c.reserve(g.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.save(path, g); err != nil {
		t.Fatal(err)
	}
}
func invokeTest(c Client, input string, args ...string) (int, string, string) {
	var out, stderr bytes.Buffer
	code := run(c, args, strings.NewReader(input), &out, &stderr)
	return code, out.String(), stderr.String()
}
func TestListIsLocalAndOldGrantSurvivesTransfer(t *testing.T) {
	c := newClient()
	c.Home = filepath.Join(t.TempDir(), "one")
	c.HTTP = &http.Client{Transport: rejectRequests{t}}
	g := Grant{Name: "old", URL: "https://lme.viibe.to/t/" + testToken + "/mcp", ExpiresAt: "2026-10-12T00:00:00Z"}
	saveTestGrant(t, c, g)
	if _, err := c.reserve("pending"); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"list"}, {"list", "--json"}} {
		code, out, stderr := invokeTest(c, "", args...)
		if code != 0 || stderr != "" || !strings.Contains(out, "old") || !strings.Contains(out, "pending") || !strings.Contains(out, testToken) {
			t.Fatal(code, out, stderr)
		}
		if len(args) == 2 && !strings.Contains(out, `"roomId":null`) {
			t.Fatal(out)
		}
	}
	_, exported, _ := invokeTest(c, "", "export", "old", "--json")
	other := c
	other.Home = filepath.Join(t.TempDir(), "two")
	if code, _, stderr := invokeTest(other, exported, "import", "renamed", "--json"); code != 0 {
		t.Fatal(stderr)
	}
	copied, err := other.load("renamed")
	if err != nil || copied.URL != g.URL || copied.RoomID != "" || copied.Name != "renamed" {
		t.Fatal(copied, err)
	}
}

type rejectRequests struct{ t *testing.T }

func (r rejectRequests) RoundTrip(*http.Request) (*http.Response, error) {
	r.t.Fatal("local command made a network request")
	return nil, fmt.Errorf("unexpected network")
}

// Reproduce 502 at each remote boundary, without assuming it deletes a Room.
func TestInspectionPartialFailuresAndCache(t *testing.T) {
	for _, failure := range []string{"", "initialize", "discovery", "identity", "state"} {
		t.Run(failure, func(t *testing.T) {
			calls := []string{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					ID     int    `json:"id"`
					Method string `json:"method"`
					Params struct {
						Name string `json:"name"`
					} `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				stage := req.Method
				var result any = map[string]any{"protocolVersion": "2025-06-18"}
				if req.Method == "tools/list" {
					stage = "discovery"
					result = map[string]any{"tools": []map[string]string{{"name": "world_list"}, {"name": "memory_state"}, {"name": "memory_search"}}}
				}
				if req.Method == "tools/call" {
					if req.Params.Name == "world_list" {
						stage = "identity"
						result = map[string]any{"structuredContent": map[string]any{"worlds": []map[string]any{{"id": testRoomID, "isDefault": true}}}}
					} else if req.Params.Name == "memory_state" {
						stage = "state"
						result = map[string]any{"structuredContent": map[string]any{"episodicCount": 2, "selfFacetCount": 0, "prospectiveCount": 0, "recentMemories": []string{"checkpoint"}}}
					} else {
						t.Errorf("unexpected tool %q", req.Params.Name)
					}
				}
				calls = append(calls, stage)
				if stage == failure {
					w.WriteHeader(502)
					fmt.Fprint(w, "upstream credential secret-do-not-print")
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
			}))
			defer server.Close()
			c := newClient()
			c.Home = filepath.Join(t.TempDir(), "private")
			saveTestGrant(t, c, Grant{Name: "demo", URL: server.URL + "/t/secret-do-not-print/mcp", ExpiresAt: "2026-10-12T00:00:00Z"})
			code, out, stderr := invokeTest(c, "", "inspect", "demo", "--json")
			var info Inspection
			if err := json.Unmarshal([]byte(out), &info); err != nil {
				t.Fatal(err, out)
			}
			if info.Name != "demo" || info.Lifecycle.ExpiresAtAtCreation == nil || info.Lifecycle.CurrentExpiresAt != nil || info.Addresses.MCP == "" {
				t.Fatal(info)
			}
			if strings.Contains(stderr, "secret-do-not-print") {
				t.Fatal(stderr)
			}
			wantCalls := 4
			if failure == "initialize" {
				wantCalls = 1
			}
			if failure == "discovery" {
				wantCalls = 2
			}
			if len(calls) != wantCalls {
				t.Fatalf("calls=%v", calls)
			}
			if failure == "" {
				if code != 0 || info.Status != "ok" || info.State.Status != "live" {
					t.Fatal(code, info, stderr)
				}
			} else {
				if code != 1 || info.Status != "partial_failure" || len(info.Errors) != 1 || info.Errors[0].Stage != failure || !strings.Contains(stderr, "502") {
					t.Fatal(code, info, stderr)
				}
			}
			cached, err := c.load("demo")
			if err != nil {
				t.Fatal(err)
			}
			if failure == "" || failure == "state" {
				if cached.RoomID != testRoomID || info.Identity.Source != "live" {
					t.Fatal("canonical ID not cached", cached.RoomID)
				}
				// Export/import retains the resource identity even when the alias changes.
				_, exported, _ := invokeTest(c, "", "export", "demo", "--json")
				other := c
				other.Home = filepath.Join(t.TempDir(), "other")
				if code, _, stderr := invokeTest(other, exported, "import", "different", "--json"); code != 0 {
					t.Fatal(stderr)
				}
				imported, err := other.load("different")
				if err != nil || imported.RoomID != testRoomID || imported.Name != "different" {
					t.Fatal(imported, err)
				}
			} else if cached.RoomID != "" {
				t.Fatal("invented ID")
			}
		})
	}
}
func TestReadOnlyInspectionDoesNotCallUnavailableTools(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		result := `{}`
		if req.Method == "tools/list" {
			result = `{"tools":[{"name":"memory_search"},{"name":"handoff_read"}]}`
		}
		if req.Method == "tools/call" {
			t.Error("called a missing tool")
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":%s}`, req.ID, result)
	}))
	defer server.Close()
	c := newClient()
	info, err := c.Inspect(Grant{Name: "read", URL: server.URL, RoomID: testRoomID})
	if err != nil || info.State.Status != "not_offered" || calls != 2 || shown(info.Identity.RoomID) != testRoomID {
		t.Fatal(info, err, calls)
	}
	c.Home = filepath.Join(t.TempDir(), "private")
	saveTestGrant(t, c, Grant{Name: "read", URL: server.URL})
	if code, _, _ := invokeTest(c, "don't store", "remember", "read"); code != 1 || calls != 4 {
		t.Fatal("write not refused")
	}
}
func TestDiscoveryDoesNotReadState(t *testing.T) {
	called := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		result := `{}`
		if req.Method == "tools/list" {
			result = `{"tools":[{"name":"world_list"},{"name":"memory_state"},{"name":"memory_search"}]}`
		}
		if req.Method == "tools/call" {
			called = append(called, req.Params.Name)
			result = `{"structuredContent":{"memories":[],"text":"none"}}`
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":%s}`, req.ID, result)
	}))
	defer server.Close()
	c := newClient()
	c.Home = filepath.Join(t.TempDir(), "private")
	saveTestGrant(t, c, Grant{Name: "demo", URL: server.URL})
	if code, _, stderr := invokeTest(c, "query", "recall", "demo", "--json"); code != 0 {
		t.Fatal(stderr)
	}
	if len(called) != 1 || called[0] != "memory_search" {
		t.Fatal(called)
	}
}

func TestFailedInspectionKeepsCachedIdentityInBothFormats(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(502); fmt.Fprint(w, "secret-token") }))
	defer s.Close()
	c := newClient()
	c.Home = filepath.Join(t.TempDir(), "private")
	saveTestGrant(t, c, Grant{Name: "known", RoomID: testRoomID, URL: s.URL + "/secret-token"})
	for _, args := range [][]string{{"inspect", "known"}, {"inspect", "known", "--json"}} {
		code, out, stderr := invokeTest(c, "", args...)
		if code != 1 || !strings.Contains(out, testRoomID) || !strings.Contains(out, "local_snapshot") || !strings.Contains(out, "partial_failure") || !strings.Contains(stderr, "initialize") || strings.Contains(stderr, "secret-token") {
			t.Fatal(code, out, stderr)
		}
	}
}

func TestInspectionDoesNotTreatInvalidDataAsLive(t *testing.T) {
	for _, tc := range []struct{ tool, result, stage string }{
		{"world_list", `{"structuredContent":{"worlds":[{"id":"ons_not-a-canonical-id","isDefault":true}]}}`, "identity"},
		{"world_list", `{"structuredContent":{"worlds":[]}}`, "identity"},
		{"memory_state", `{"structuredContent":{"episodicCount":-1,"selfFacetCount":0,"prospectiveCount":0}}`, "state"},
		{"memory_state", `{"structuredContent":{}}`, "state"},
		{"memory_state", `{"isError":true,"content":[{"type":"text","text":"secret-error"}]}`, "state"},
	} {
		t.Run(tc.result, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					ID     int    `json:"id"`
					Method string `json:"method"`
				}
				_ = json.NewDecoder(r.Body).Decode(&req)
				result := `{}`
				if req.Method == "tools/list" {
					result = fmt.Sprintf(`{"tools":[{"name":%q}]}`, tc.tool)
				}
				if req.Method == "tools/call" {
					result = tc.result
				}
				fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":%s}`, req.ID, result)
			}))
			defer s.Close()
			info, err := newClient().Inspect(Grant{URL: s.URL, RoomID: testRoomID})
			if err == nil || len(info.Errors) != 1 || info.Errors[0].Stage != tc.stage || strings.Contains(err.Error(), "secret-error") || info.Identity.Source == "live" || info.State.Status == "live" {
				t.Fatal(info, err)
			}
		})
	}
}
