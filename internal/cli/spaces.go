package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type Space struct {
	ID        *string `json:"id"`
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	Access    string  `json:"access"`
	Lifecycle string  `json:"lifecycle"`
	State     string  `json:"state"`
	Endpoint  string  `json:"endpoint,omitempty"`
}

type accountInventory struct {
	Entitled *bool   `json:"entitled"`
	Spaces   []Space `json:"spaces"`
}

func (c Client) fetchSpaces(a *account) (accountInventory, error) {
	var inventory accountInventory
	connected := c.withAccount(a)
	req, _ := http.NewRequest(http.MethodGet, accountSpaces, nil)
	res, err := connected.HTTP.Do(req)
	if err != nil {
		return inventory, errors.New("World inventory unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode == 401 {
		return inventory, errors.New("account authorization expired; run lm login")
	}
	if res.StatusCode != 200 {
		return inventory, fmt.Errorf("World inventory unavailable (HTTP %d)", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if err != nil || len(data) > maxResponse || json.Unmarshal(data, &inventory) != nil || inventory.Spaces == nil {
		return inventory, errors.New("invalid World inventory")
	}
	for _, s := range inventory.Spaces {
		if s.Type != "world" || !roomIDPattern.MatchString(shown(s.ID)) || s.Endpoint != accountResource || s.Access != "owner" || s.Lifecycle != "subscription" ||
			(s.State != "active" && s.State != "needs_subscription" && s.State != "unknown") {
			return accountInventory{}, errors.New("invalid World inventory")
		}
	}
	return inventory, nil
}

func (c Client) accountSpacesIfSaved() (accountInventory, string) {
	path, err := c.accountPath()
	if err != nil {
		return accountInventory{}, "local account unavailable"
	}
	if _, err = os.Stat(path); os.IsNotExist(err) {
		return accountInventory{Spaces: []Space{}}, "signed_out"
	}
	unlock, err := c.lockAccount()
	if err != nil {
		return accountInventory{}, err.Error()
	}
	defer unlock()
	a, err := c.readAccount()
	if err != nil {
		return accountInventory{}, err.Error()
	}
	inv, err := c.fetchSpaces(&a)
	if err != nil {
		return accountInventory{}, err.Error()
	}
	return inv, "ok"
}

func (c Client) localRooms() ([]RoomSummary, error) {
	if _, err := c.storePath("check"); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(c.Home)
	if err != nil {
		return nil, errors.New("cannot read local inventory")
	}
	rows := []RoomSummary{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".grant.json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".grant.json")
		g, err := c.load(name)
		if err != nil {
			row := summary(Grant{Name: name, Kind: "room"})
			row.Saved = false
			row.State = "unavailable_or_pending"
			row.Space.State = "needs_attention"
			row.Next = "Check the saved grant; do not blindly recreate this Room."
			rows = append(rows, row)
		} else {
			rows = append(rows, summary(g))
		}
	}
	return rows, nil
}

func roomSpace(row RoomSummary) Space {
	state := "saved"
	if !row.Saved {
		state = "needs_attention"
	}
	return Space{ID: row.Identity.RoomID, Name: row.Name, Type: "room", Access: row.Addresses.Access, Lifecycle: "inactivity_expiry", State: state}
}

func spaceCommand(command string) bool {
	switch command {
	case "inspect", "state", "remember", "recall", "handoff", "resume":
		return true
	}
	return false
}

// A bare name is convenient only when it resolves unambiguously. Explicit
// room:/world: selectors remain usable when the account inventory is offline.
func (c Client) resolveSelector(raw string) (alias, worldID string, isWorld bool, err error) {
	if strings.HasPrefix(raw, "room:") {
		alias = strings.TrimPrefix(raw, "room:")
		if !aliasPattern.MatchString(alias) {
			err = errors.New("invalid Room selector")
		}
		return
	}
	if strings.HasPrefix(raw, "world:") {
		worldID = strings.TrimPrefix(raw, "world:")
		if !roomIDPattern.MatchString(worldID) {
			err = errors.New("invalid World ID")
		}
		isWorld = true
		return
	}
	if strings.TrimSpace(raw) == "" || len(raw) > 120 || strings.ContainsAny(raw, "\r\n\x1b") {
		err = errors.New("invalid Space name; use room:<alias> or world:<id>")
		return
	}
	localSaved := false
	if aliasPattern.MatchString(raw) {
		path, pathErr := c.storePath(raw)
		if pathErr == nil {
			_, statErr := os.Lstat(path)
			localSaved = statErr == nil
		}
	}
	inv, status := c.accountSpacesIfSaved()
	if status != "ok" && status != "signed_out" {
		err = errors.New("World inventory unavailable; use room:<alias> for a local Room")
		return
	}
	matches := []Space{}
	for _, s := range inv.Spaces {
		if strings.EqualFold(s.Name, raw) {
			matches = append(matches, s)
		}
	}
	if len(matches) > 1 || (localSaved && len(matches) > 0) {
		err = errors.New("Space name is ambiguous; use room:<alias> or world:<id> from lm list --json")
		return
	}
	if len(matches) == 1 {
		return "", shown(matches[0].ID), true, nil
	}
	if !aliasPattern.MatchString(raw) {
		return "", "", false, errors.New("no accessible World has that name; run lm list")
	}
	return raw, "", false, nil
}

func privateListRoom(row RoomSummary) RoomSummary {
	row.Space.Endpoint = ""
	row.Addresses.Open = nil
	row.Addresses.Guide = nil
	row.Addresses.MCP = ""
	row.Addresses.Warning = "Run lm inspect room:" + row.Name + " for entrances."
	return row
}

func (c Client) listSpaces(out io.Writer, asJSON bool) error {
	rooms, err := c.localRooms()
	if err != nil {
		return err
	}
	remote, remoteStatus := c.accountSpacesIfSaved()
	spaces := make([]Space, 0, len(rooms)+len(remote.Spaces))
	privateRooms := make([]RoomSummary, 0, len(rooms))
	for _, row := range rooms {
		spaces = append(spaces, roomSpace(row))
		privateRooms = append(privateRooms, privateListRoom(row))
	}
	spaces = append(spaces, remote.Spaces...)
	for i := range spaces {
		spaces[i].Endpoint = ""
	}
	status := "ok"
	if remoteStatus != "ok" && remoteStatus != "signed_out" {
		status = "partial_failure"
	}
	if asJSON {
		return json.NewEncoder(out).Encode(map[string]any{"scope": "local_and_account", "rooms": privateRooms,
			"spaces": spaces, "remoteStatus": remoteStatus, "status": status, "entitled": remote.Entitled})
	}
	if _, err := fmt.Fprintln(out, "Spaces\n  NAME                  TYPE    ACCESS       STATUS"); err != nil {
		return err
	}
	for _, s := range spaces {
		name := strings.Join(strings.Fields(terminalText(s.Name)), " ")
		if _, err := fmt.Fprintf(out, "  %-20s  %-6s  %-11s  %s\n", name, s.Type, s.Access, s.State); err != nil {
			return err
		}
	}
	if len(spaces) == 0 && status == "ok" {
		if remoteStatus == "signed_out" {
			fmt.Fprintln(out, "  No Spaces yet. Create a Room: lm create my-room --room  ·  Sign in for Worlds: lm login")
		} else {
			fmt.Fprintln(out, "  No Spaces yet. Create a Room: lm create my-room --room")
		}
	}
	if status != "ok" {
		fmt.Fprintln(out, "  World could not be loaded. Local Rooms are shown; try again or use room:<alias>.")
	} else if remoteStatus == "ok" && len(remote.Spaces) == 0 {
		fmt.Fprintln(out, "  No World on this account. Next: lm world")
	} else {
		for _, s := range remote.Spaces {
			if s.State == "needs_subscription" {
				fmt.Fprintln(out, "  World subscription needed. Next: lm world")
				break
			}
			if s.State == "unknown" {
				fmt.Fprintln(out, "  World billing status is unknown. Next: lm world")
				break
			}
		}
	}
	if len(spaces) == 0 {
		return nil
	}
	_, err = fmt.Fprintln(out, "  Details: lm inspect <name>  (quote names with spaces; use room:<alias> or world:<id> for collisions)")
	return err
}

func (c Client) world(out, stderr io.Writer, asJSON bool) int {
	fail := func(err error) int { fmt.Fprintln(stderr, "lm:", err); return 1 }
	path, err := c.accountPath()
	if err != nil {
		return fail(err)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return worldDirection(out, stderr, asJSON, "signed_out")
	}
	unlock, err := c.lockAccount()
	if err != nil {
		return fail(err)
	}
	defer unlock()
	a, err := c.readAccount()
	if err != nil {
		return fail(err)
	}
	inv, err := c.fetchSpaces(&a)
	if err != nil {
		return fail(err)
	}
	if inv.Entitled == nil {
		return worldDirection(out, stderr, asJSON, "billing_unknown")
	}
	if !*inv.Entitled {
		return worldDirection(out, stderr, asJSON, "needs_subscription")
	}
	// Provisioning is deliberate here, never in login/list: the OAuth MCP rail
	// creates a first World on initialize for an entitled account.
	if _, err := c.withAccount(&a).Discover(Grant{URL: accountResource}); err != nil {
		return fail(fmt.Errorf("World access could not be activated: %w", err))
	}
	inv, err = c.fetchSpaces(&a)
	if err != nil || len(inv.Spaces) == 0 {
		return fail(errors.New("World access opened but inventory could not confirm its identity; run lm list before retrying"))
	}
	if asJSON {
		if err := json.NewEncoder(out).Encode(map[string]any{"status": "active", "spaces": inv.Spaces, "next": "lm list"}); err != nil {
			return fail(errors.New("output failed"))
		}
	} else {
		if _, err := fmt.Fprintf(out, "World ready: %s\nNext: lm list, then lm inspect world:%s\n", strings.Join(strings.Fields(terminalText(inv.Spaces[0].Name)), " "), shown(inv.Spaces[0].ID)); err != nil {
			return fail(errors.New("output failed"))
		}
	}
	return 0
}

func worldDirection(out, stderr io.Writer, asJSON bool, status string) int {
	fail := func() int { fmt.Fprintln(stderr, "lm: output failed"); return 1 }
	next := "lm world"
	if status == "signed_out" {
		next = "lm login"
	}
	if asJSON {
		var url *string
		if status != "billing_unknown" {
			url = optional(worldURL)
		}
		if json.NewEncoder(out).Encode(map[string]any{"status": status, "url": url, "terms": termsURL, "privacy": privacyURL, "next": next, "roomMigration": false}) != nil {
			return fail()
		}
		return 0
	}
	if status == "billing_unknown" {
		if _, err := fmt.Fprintln(out, "Billing status is unavailable. Do not purchase again until it can be checked.\nTry: lm world"); err != nil {
			return fail()
		}
		return 0
	}
	var err error
	if status == "signed_out" {
		_, err = fmt.Fprintln(out, "Sign in first: lm login")
	} else {
		_, err = fmt.Fprintln(out, "No active World subscription on this sign-in. If you just paid, allow a minute and run lm world again.")
	}
	if err != nil {
		return fail()
	}
	if _, err = fmt.Fprintf(out, "Plan and checkout: %s\nTerms: %s\nPrivacy: %s\nAfter checkout, return here and run: lm world\nYour Rooms stay separate.\n", worldURL, termsURL, privacyURL); err != nil {
		return fail()
	}
	return 0
}
