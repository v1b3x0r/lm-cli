package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
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
	saveTestGrant(t, c, Grant{Name: "journal", Kind: "room", RoomID: testRoomID, URL: "https://lme.viibe.to/t/" + testToken + "/mcp", ExpiresAt: "2026-10-12T00:00:00Z"})
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
}

func TestWorldHumanTitleWithSpacesSelectsDirectly(t *testing.T) {
	yes := true
	c, _ := spaceClient(t, &yes, true, "Cycling Series")
	_, id, world, err := c.resolveSelector("Cycling Series")
	if err != nil || !world || id != testRoomID {
		t.Fatal(id, world, err)
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
