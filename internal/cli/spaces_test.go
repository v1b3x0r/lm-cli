package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func spaceClient(t *testing.T, entitled *bool, initialWorld bool, worldName ...string) (Client, *bool) {
	t.Helper()
	c := newClient()
	c.Home = filepath.Join(t.TempDir(), "private")
	if err := c.saveAccount(account{ClientID: "client", Access: "secret-account-token", Expires: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	active := new(bool)
	*active = initialWorld
	name := "journal"
	if len(worldName) > 0 {
		name = worldName[0]
	}
	c.HTTP = &http.Client{Transport: authRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer secret-account-token" {
			t.Fatal("missing bearer")
		}
		switch r.URL.String() {
		case accountSpaces:
			spaces := []Space{}
			if *active {
				spaces = append(spaces, Space{ID: optional(testRoomID), Name: name, Type: "world", Access: "owner", Lifecycle: "subscription", State: "active", Endpoint: accountResource})
			}
			b, _ := json.Marshal(accountInventory{Entitled: entitled, Spaces: spaces})
			return authResponse(200, string(b)), nil
		case accountResource:
			if entitled == nil || !*entitled {
				t.Fatal("MCP called without entitlement")
			}
			*active = true
			var req struct {
				ID     int    `json:"id"`
				Method string `json:"method"`
				Params struct {
					Arguments map[string]any `json:"arguments"`
				} `json:"params"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			result := `{"protocolVersion":"2025-06-18"}`
			if req.Method == "tools/list" {
				result = `{"tools":[{"name":"memory_state"}]}`
			}
			if req.Method == "tools/call" {
				if req.Params.Arguments["world_id"] != testRoomID {
					t.Fatal("world selector not forwarded")
				}
				result = `{"structuredContent":{"episodicCount":1,"selfFacetCount":0,"prospectiveCount":0}}`
			}
			b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": json.RawMessage(result)})
			return authResponse(200, string(b)), nil
		}
		t.Fatal("unexpected destination")
		return authResponse(500, ""), nil
	})}
	return c, active
}

func TestSpacesListAndSelectorCollision(t *testing.T) {
	yes := true
	c, _ := spaceClient(t, &yes, true)
	owner := "https://lme.viibe.to/t/" + testToken + "/mcp"
	saveTestGrant(t, c, Grant{Name: "journal", Kind: "room", RoomID: testRoomID, URL: owner, ExpiresAt: "2026-10-12T00:00:00Z"})
	for _, args := range [][]string{{"list"}, {"list", "--json"}} {
		code, out, stderr := invokeTest(c, "", args...)
		if code != 0 || stderr != "" || strings.Count(out, "journal") < 2 || strings.Contains(out, testToken) || strings.Contains(out, "secret-account-token") || strings.Contains(out, accountResource) {
			t.Fatal(code, out, stderr)
		}
	}
	if _, _, _, err := c.resolveSelector("journal"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatal(err)
	}
	if alias, _, world, err := c.resolveSelector("room:journal"); err != nil || alias != "journal" || world {
		t.Fatal(alias, world, err)
	}
	if _, id, world, err := c.resolveSelector("world:" + testRoomID); err != nil || id != testRoomID || !world {
		t.Fatal(id, world, err)
	}
	code, out, stderr := invokeTest(c, "", "state", "world:"+testRoomID, "--json")
	if code != 0 || stderr != "" || !strings.Contains(out, "episodicCount") {
		t.Fatal(code, out, stderr)
	}
	base := c.HTTP.Transport
	selectedTool := ""
	c.HTTP.Transport = authRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != owner {
			return base.RoundTrip(r)
		}
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		result := map[string]any{"protocolVersion": "2025-06-18"}
		if req.Method == "tools/list" {
			result = map[string]any{"tools": []map[string]string{{"name": selectedTool}}}
		} else if req.Method == "tools/call" {
			result = map[string]any{"structuredContent": map[string]int{"episodicCount": 1, "selfFacetCount": 0, "prospectiveCount": 0}}
		}
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		return authResponse(200, string(body)), nil
	})
	for _, tc := range []struct{ tool, next string }{{"handoff_read", "lm resume room:journal"}, {"memory_state", "lm state room:journal"}} {
		selectedTool = tc.tool
		code, out, stderr = invokeTest(c, "", "inspect", "room:journal", "--json")
		if code != 0 || stderr != "" || !strings.Contains(out, `"next":"`+tc.next+`"`) {
			t.Fatal(tc, code, out, stderr)
		}
	}
	if next := summary(Grant{Name: "journal", Kind: "room", URL: owner}).Next; next != "lm inspect room:journal" {
		t.Fatal(next)
	}
}

func TestWorldHumanTitleWithSpacesSelectsDirectly(t *testing.T) {
	yes := true
	c, _ := spaceClient(t, &yes, true, "Cycling Series")
	_, id, world, err := c.resolveSelector("Cycling Series")
	if err != nil || !world || id != testRoomID {
		t.Fatal(id, world, err)
	}
}

func TestExplicitWorldRefreshesBeforeInventoryAndMCP(t *testing.T) {
	c := newClient()
	c.Home = filepath.Join(t.TempDir(), "private")
	if err := c.saveAccount(account{ClientID: "client", Access: "expired", Refresh: "old-refresh", Expires: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	refreshes, inventories, mcpCalls := 0, 0, 0
	c.HTTP = &http.Client{Transport: authRoundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.URL.String() {
		case accountIssuer + "/v1/oauth2/token":
			refreshes++
			body, _ := io.ReadAll(r.Body)
			values, _ := url.ParseQuery(string(body))
			if values.Get("refresh_token") != "old-refresh" || r.Header.Get("Authorization") != "" {
				t.Fatal("incorrect refresh request")
			}
			return authResponse(200, `{"access_token":"fresh","refresh_token":"rotated","token_type":"Bearer","expires_in":3600}`), nil
		case accountSpaces:
			inventories++
			if r.Header.Get("Authorization") != "Bearer fresh" {
				t.Fatal("inventory used stale token")
			}
			return authResponse(200, `{"entitled":true,"spaces":[{"id":"`+testRoomID+`","name":"journal","type":"world","access":"owner","lifecycle":"subscription","state":"active","endpoint":"`+accountResource+`"}]}`), nil
		case accountResource:
			mcpCalls++
			if r.Header.Get("Authorization") != "Bearer fresh" {
				t.Fatal("MCP used stale token")
			}
			var req struct {
				ID     int    `json:"id"`
				Method string `json:"method"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			result := `{"protocolVersion":"2025-06-18"}`
			if req.Method == "tools/list" {
				result = `{"tools":[]}`
			}
			return authResponse(200, `{"jsonrpc":"2.0","id":`+strconv.Itoa(req.ID)+`,"result":`+result+`}`), nil
		}
		t.Fatal("unexpected destination")
		return authResponse(500, ""), nil
	})}
	code, out, stderr := invokeTest(c, "", "inspect", "world:"+testRoomID, "--json")
	if code != 0 || stderr != "" || refreshes != 1 || inventories != 1 || mcpCalls != 2 || !strings.Contains(out, `"next":"lm state world:`+testRoomID+`"`) {
		t.Fatal(code, out, stderr, refreshes, inventories, mcpCalls)
	}
	saved, err := c.readAccount()
	if err != nil || saved.Refresh != "rotated" {
		t.Fatal("refresh was not persisted", err)
	}
}

func TestWorldInspectionNextKeepsSelectedID(t *testing.T) {
	yes := true
	c, _ := spaceClient(t, &yes, true)
	code, out, stderr := invokeTest(c, "", "inspect", "world:"+testRoomID, "--json")
	if code != 0 || stderr != "" || !strings.Contains(out, `"next":"lm state world:`+testRoomID+`"`) || !strings.Contains(out, `"roomId":"`+testRoomID+`"`) {
		t.Fatal(code, out, stderr)
	}
	if next := summary(Grant{Kind: "world", URL: accountResource}).Next; next != "lm state --account" {
		t.Fatal(next)
	}
}

func TestHumanRoomSummaryKeepsEntrancesAndReservesDetailForInspect(t *testing.T) {
	owner := "https://lme.viibe.to/t/" + testToken + "/mcp"
	public := "https://lme.viibe.to/t/ro_abcdef0123456789abcdef0123456789/mcp"
	s := summary(Grant{Name: "ride", Kind: "room", RoomID: testRoomID, URL: owner, ReadOnlyURL: public, ExpiresAt: "2026-10-12T00:00:00Z"})
	var out bytes.Buffer
	if err := renderSummary(&out, s, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Guide") || !strings.Contains(out.String(), "https://living-memory.app/theatre#ro_") || !strings.Contains(out.String(), owner) || strings.Contains(out.String(), testRoomID) || strings.Contains(out.String(), "T00:00:00") {
		t.Fatal(out.String())
	}
	out.Reset()
	if err := renderSummary(&out, s, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), testRoomID) || !strings.Contains(out.String(), "2026-10-12T00:00:00Z") {
		t.Fatal(out.String())
	}
}

func TestWorldPurchaseReturnAndUnknownBilling(t *testing.T) {
	no := false
	c, active := spaceClient(t, &no, false)
	code, out, stderr := invokeTest(c, "", "world")
	if code != 0 || stderr != "" || !strings.Contains(out, worldURL) || !strings.Contains(out, termsURL) || !strings.Contains(out, privacyURL) || *active {
		t.Fatal(code, out, stderr)
	}
	yes := true
	c2, activated := spaceClient(t, &yes, false)
	code, out, stderr = invokeTest(c2, "", "world")
	if code != 0 || stderr != "" || !*activated || !strings.Contains(out, "World ready") || !strings.Contains(out, "lm inspect world:") {
		t.Fatal(code, out, stderr)
	}
	c3, _ := spaceClient(t, nil, false)
	code, out, stderr = invokeTest(c3, "", "world")
	if code != 0 || stderr != "" || !strings.Contains(out, "Do not purchase again") || strings.Contains(out, "No active World subscription") {
		t.Fatal(code, out, stderr)
	}
}

func TestListKeepsRoomsOnWorldFailure(t *testing.T) {
	yes := true
	c, _ := spaceClient(t, &yes, false)
	saveTestGrant(t, c, Grant{Name: "local", Kind: "room", URL: "https://lme.viibe.to/t/" + testToken + "/mcp"})
	c.HTTP = &http.Client{Transport: authRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != accountSpaces {
			t.Fatal("unexpected request")
		}
		return authResponse(503, `{"error":"no"}`), nil
	})}
	code, out, stderr := invokeTest(c, "", "list", "--json")
	if code != 0 || stderr != "" || !strings.Contains(out, `"status":"partial_failure"`) || !strings.Contains(out, "local") || strings.Contains(out, testToken) {
		t.Fatal(code, out, stderr)
	}
	if _, _, _, err := c.resolveSelector("local"); err == nil {
		t.Fatal("bare selector guessed while offline")
	}
	if alias, _, _, err := c.resolveSelector("room:local"); err != nil || alias != "local" {
		t.Fatal(alias, err)
	}
}
