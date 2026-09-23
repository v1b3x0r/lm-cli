package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRenameRoomAliasPreservesGrantAndCollisionHandling(t *testing.T) {
	yes := true
	c, _ := spaceClient(t, &yes, true, "journal")
	owner := "https://lme.viibe.to/t/" + testToken + "/mcp"
	public := "https://lme.viibe.to/t/ro_0123456789abcdef0123456789abcdef/mcp"
	saveTestGrant(t, c, Grant{Name: "old", Kind: "room", URL: owner, ReadOnlyURL: public, RoomID: testRoomID, ExpiresAt: "2026-10-12T00:00:00Z"})
	oldPath, _ := c.storePath("old")
	before, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	code, out, stderr := invokeTest(c, "", "rename", "room:old", "journal", "--json")
	if code != 0 || stderr != "" || !strings.Contains(out, `"alias":"journal"`) || strings.Contains(out, testToken) {
		t.Fatal(code, out, stderr)
	}
	newPath, _ := c.storePath("journal")
	after, err := os.ReadFile(newPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("rename changed grant bytes", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatal("old alias still exists", err)
	}
	g, err := c.load("journal")
	if err != nil || g.Name != "journal" || g.URL != owner || g.ReadOnlyURL != public || g.RoomID != testRoomID {
		t.Fatal("renamed grant lost identity or doors", err)
	}
	if _, _, _, err := c.resolveSelector("journal"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatal("Room/World name collision was not preserved", err)
	}
	if alias, _, world, err := c.resolveSelector("room:journal"); err != nil || world || alias != "journal" {
		t.Fatal(alias, world, err)
	}
	if _, id, world, err := c.resolveSelector("world:" + testRoomID); err != nil || !world || id != testRoomID {
		t.Fatal(id, world, err)
	}
}

func TestRenameRefusesExistingAliasWorldAndInvalidName(t *testing.T) {
	yes := true
	c, _ := spaceClient(t, &yes, true, "journal")
	owner := "https://lme.viibe.to/t/" + testToken + "/mcp"
	saveTestGrant(t, c, Grant{Name: "old", Kind: "room", URL: owner})
	saveTestGrant(t, c, Grant{Name: "taken", Kind: "room", URL: owner})
	for _, args := range [][]string{
		{"rename", "room:old", "taken"},
		{"rename", "room:old", "bad/name"},
		{"rename", "room:old", "old"},
		{"rename", "world:" + testRoomID, "other"},
		{"rename", "journal", "other"},
		{"rename", "room:old", "other", "--account"},
	} {
		if code, _, _ := invokeTest(c, "", args...); code == 0 {
			t.Fatalf("accepted %v", args)
		}
	}
	if _, err := c.load("old"); err != nil {
		t.Fatal("source changed after rejected rename", err)
	}
	if _, err := c.load("taken"); err != nil {
		t.Fatal("target changed after rejected rename", err)
	}
}

func TestRenameLegacyRoomWorksOfflineWithoutInventingGuide(t *testing.T) {
	c := newClient()
	c.Home = t.TempDir() + "/private"
	owner := "https://lme.viibe.to/t/" + testToken + "/mcp"
	saveTestGrant(t, c, Grant{Name: "old", Kind: "room", URL: owner, RoomID: testRoomID})
	code, out, stderr := invokeTest(c, "", "rename", "room:old", "better")
	if code != 0 || stderr != "" || !strings.Contains(out, "lm inspect room:better") || strings.Contains(out, testToken) {
		t.Fatal(code, out, stderr)
	}
	g, err := c.load("better")
	if err != nil {
		t.Fatal(err)
	}
	a := summary(g).Addresses
	if a.Open != nil || a.Guide != nil || a.MCP != owner {
		t.Fatal("legacy grant gained an invented public door")
	}
}
