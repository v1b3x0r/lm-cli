package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const worldURL = "https://living-memory.app/create#world"

func (c Client) extra(command string, args []string, room, asJSON bool, in io.Reader, out, stderr io.Writer) int {
	fail := func(err error) int { fmt.Fprintln(stderr, "lm:", err); return 1 }
	emit := func(value any) int {
		if err := json.NewEncoder(out).Encode(value); err != nil {
			return fail(errors.New("output failed"))
		}
		return 0
	}
	if room {
		return fail(errors.New("--room is only valid for create"))
	}
	if command == "world" {
		if len(args) != 0 {
			return fail(errors.New("usage: lm world [--json]"))
		}
		info := map[string]any{"url": worldURL, "next": "Sign in on the website, review the current plan, and complete checkout there. Already subscribed? Use the existing World setup.", "roomMigration": false, "note": "Your Room remains separate. This command does not buy anything or confirm payment."}
		if asJSON {
			return emit(info)
		}
		_, err := fmt.Fprintf(out, "Continue to your World: %s\nSign in, review the current plan, and complete checkout on the website.\nAlready subscribed? Use your existing World setup.\nYour Room remains separate; automatic migration and CLI payment confirmation are not available.\n", worldURL)
		if err != nil {
			return fail(errors.New("output failed"))
		}
		return 0
	}
	if command == "list" {
		if len(args) != 0 {
			return fail(errors.New("usage: lm list [--json]"))
		}
		if _, err := c.storePath("check"); err != nil {
			return fail(err)
		}
		entries, err := os.ReadDir(c.Home)
		if err != nil {
			return fail(errors.New("cannot read local inventory"))
		}
		rows := []RoomSummary{}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".grant.json") {
				name := strings.TrimSuffix(e.Name(), ".grant.json")
				g, err := c.load(name)
				if err != nil {
					row := summary(Grant{Name: name})
					row.Saved = false
					row.State = "unavailable_or_pending"
					row.Next = "Check the saved grant; do not blindly recreate this Room."
					rows = append(rows, row)
				} else {
					rows = append(rows, summary(g))
				}
			}
		}
		if asJSON {
			return emit(map[string]any{"scope": "local", "rooms": rows})
		}
		if _, err := fmt.Fprintln(out, "Local Rooms (saved snapshots; no network lookup):"); err != nil {
			return fail(errors.New("output failed"))
		}
		if len(rows) == 0 {
			_, err = fmt.Fprintln(out, "No Rooms saved here. Next: lm create my-room --room")
		}
		for _, row := range rows {
			if err == nil {
				err = renderSummary(out, row)
			}
		}
		if err != nil {
			return fail(errors.New("output failed"))
		}
		return 0
	}
	tool := map[string]string{"remember": "memory_add", "recall": "memory_search", "handoff": "handoff_post", "resume": "handoff_read", "state": "memory_state"}[command]
	if tool == "" && command != "export" && command != "import" {
		return fail(errors.New("unsupported command; run lm help"))
	}
	if len(args) != 1 {
		return fail(errors.New("command requires one local room name; run lm help"))
	}
	if command == "import" {
		data, err := io.ReadAll(io.LimitReader(in, maxResponse+1))
		var g Grant
		if err != nil || len(data) > maxResponse || json.Unmarshal(data, &g) != nil || !validAddress(g.URL) {
			return fail(errors.New("stdin must contain a valid room grant"))
		}
		g.Name = args[0]
		g.Kind = "room"
		g.Warning = addresses(g.URL, g.ReadOnlyURL).Warning
		if !roomIDPattern.MatchString(g.RoomID) {
			g.RoomID = ""
		}
		path, err := c.reserve(g.Name)
		if err != nil {
			return fail(err)
		}
		if err = c.save(path, g); err != nil {
			return fail(err)
		}
		return emit(summary(g))
	}
	if command == "export" && !asJSON {
		return fail(errors.New("export requires --json; output is a secret grant, save or transfer it privately"))
	}
	g, err := c.load(args[0])
	if err != nil {
		return fail(err)
	}
	if command == "export" {
		return emit(g)
	}
	params := map[string]any{}
	if command == "remember" || command == "recall" || command == "handoff" {
		data, err := io.ReadAll(io.LimitReader(in, 65537))
		if err != nil || len(data) > 65536 || strings.TrimSpace(string(data)) == "" {
			return fail(errors.New("stdin must contain 1–65536 bytes of text"))
		}
		key := map[string]string{"remember": "content", "recall": "query", "handoff": "text"}[command]
		params[key] = string(data)
		if command == "handoff" {
			params["from"] = "lm-cli"
		}
	}
	// Discover before calling: read-only doors and unavailable tools are real server boundaries.
	inventory, err := c.Discover(g)
	if err != nil {
		return fail(err)
	}
	available := false
	for _, name := range inventory.Tools {
		if name == tool {
			available = true
		}
	}
	if !available {
		return fail(errors.New("this door does not offer the requested tool"))
	}
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Structured json.RawMessage `json:"structuredContent"`
		IsError    bool            `json:"isError"`
	}
	if err = c.rpc(g.URL, "tools/call", map[string]any{"name": tool, "arguments": params}, 102, &result); err != nil {
		return fail(err)
	}
	if result.IsError {
		return fail(errors.New("tool rejected the operation; no automatic retry"))
	}
	if len(result.Structured) == 0 && len(result.Content) == 0 {
		return fail(errors.New("invalid tool result; outcome unknown"))
	}
	if asJSON {
		if len(result.Structured) > 0 {
			return emit(result.Structured)
		}
		return emit(map[string]any{"content": result.Content})
	}
	for _, item := range result.Content {
		if item.Type == "text" {
			if _, err = fmt.Fprintln(out, terminalText(item.Text)); err != nil {
				return fail(errors.New("output failed"))
			}
		}
	}
	return 0
}

// Memory is untrusted text; avoid interpreting terminal escape/control sequences.
func terminalText(s string) string {
	return strings.Map(func(r rune) rune {
		if (r < 32 && r != '\n' && r != '\t') || (r >= 127 && r <= 159) {
			return -1
		}
		return r
	}, s)
}
