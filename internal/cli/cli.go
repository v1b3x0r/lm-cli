package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var version = "0.1.0-rc.4"

const maxResponse = 2 << 20

type Grant struct {
	Name        string `json:"name"`
	RoomID      string `json:"roomId,omitempty"`
	Kind        string `json:"kind"`
	URL         string `json:"url"`
	ReadOnlyURL string `json:"readOnlyUrl,omitempty"`
	ExpiresAt   string `json:"expiresAt"`
	Note        string `json:"note,omitempty"`
	Warning     string `json:"warning"`
}

type Client struct {
	HTTP         *http.Client
	Base         string
	Home         string
	accountGrant *Grant
	openBrowser  func(string)
	loginTimeout time.Duration
}

func newClient() Client {
	home := os.Getenv("LM_HOME")
	if home == "" {
		if root, err := os.UserConfigDir(); err == nil {
			home = filepath.Join(root, "lm-cli")
		}
	}
	return Client{Home: home, HTTP: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, Base: "https://lme.viibe.to"}
}

// Errors intentionally omit URLs, response bodies and transport errors: each can contain a credential.
func (c Client) post(address string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, address, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("invalid endpoint")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	res, err := c.HTTP.Do(req)
	if err != nil {
		if c.accountGrant != nil {
			return nil, errors.New("account request or refresh failed; run lm login to reconnect. Outcome unknown, no automatic retry")
		}
		return nil, errors.New("request failed or timed out; outcome unknown, no automatic retry")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		if c.accountGrant != nil && res.StatusCode == 401 {
			return nil, errors.New("account authorization rejected; run lm login. Operation was not retried")
		}
		if c.accountGrant != nil && res.StatusCode == 403 {
			return nil, errors.New("World access denied; run lm world to check the account subscription. Operation was not retried")
		}
		data, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		var problem struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(data, &problem)
		if res.StatusCode == 429 && problem.Code == "ROOM_FULL" {
			return nil, errors.New("ROOM_FULL: existing memories remain readable. For a persistent World, run lm world. Room migration is not automatic")
		}
		return nil, fmt.Errorf("server answered HTTP %d; no automatic retry", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if err != nil || len(data) > maxResponse {
		return nil, errors.New("response unreadable or too large; outcome unknown")
	}
	return data, nil
}

func validAddress(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	return u.Scheme == "https" || (u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1"))
}

func (c Client) Create(name string) (Grant, error) {
	data, err := c.post(c.Base+"/ons/new", nil)
	if err != nil {
		return Grant{}, err
	}
	var g Grant
	if json.Unmarshal(data, &g) != nil || !validAddress(g.URL) {
		return Grant{}, errors.New("invalid room grant; creation outcome unknown, do not blindly retry")
	}
	if _, err = time.Parse(time.RFC3339, g.ExpiresAt); err != nil {
		return Grant{}, errors.New("invalid expiry in room grant; creation outcome unknown")
	}
	g.Name = name
	g.Kind = "room"
	g.Warning = addresses(g.URL, g.ReadOnlyURL).Warning
	if !roomIDPattern.MatchString(g.RoomID) {
		g.RoomID = ""
	}
	return g, nil
}

func (c Client) rpc(address, method string, params any, id int, result any) error {
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	data, err := c.post(address, body)
	if err != nil {
		return err
	}
	// Streamable HTTP can return one JSON envelope or SSE frames. Join multiline data fields.
	candidates := [][]byte{data}
	if !json.Valid(data) {
		candidates = nil
		for _, frame := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n\n") {
			var lines []string
			for _, line := range strings.Split(frame, "\n") {
				if strings.HasPrefix(line, "data:") {
					lines = append(lines, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
				}
			}
			if len(lines) > 0 {
				candidates = append(candidates, []byte(strings.Join(lines, "\n")))
			}
		}
	}
	for _, candidate := range candidates {
		var envelope struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      int             `json:"id"`
			Result  json.RawMessage `json:"result"`
			Error   json.RawMessage `json:"error"`
		}
		if json.Unmarshal(candidate, &envelope) != nil || envelope.JSONRPC != "2.0" || envelope.ID != id {
			continue
		}
		if len(envelope.Error) > 0 && string(envelope.Error) != "null" {
			return errors.New("MCP request rejected")
		}
		if len(envelope.Result) == 0 || string(envelope.Result) == "null" || json.Unmarshal(envelope.Result, result) != nil {
			return errors.New("invalid MCP result")
		}
		return nil
	}
	return errors.New("invalid MCP response")
}

type Discovery struct {
	Name  string   `json:"name"`
	Tools []string `json:"tools"`
}

func (c Client) Discover(g Grant) (Discovery, error) {
	out := Discovery{Name: g.Name, Tools: []string{}}
	if !validAddress(g.URL) {
		return out, errors.New("grant requires a HTTPS room URL (HTTP allowed only on loopback)")
	}
	var initialized map[string]any
	err := c.rpc(g.URL, "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "lm-cli", "version": version}}, 1, &initialized)
	if err != nil {
		return out, fmt.Errorf("initialize: %w", err)
	}
	cursor := ""
	seen := map[string]bool{}
	for id := 2; id <= 101; id++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var page struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
			NextCursor string `json:"nextCursor"`
		}
		if err = c.rpc(g.URL, "tools/list", params, id, &page); err != nil {
			return out, fmt.Errorf("discovery: %w", err)
		}
		if page.Tools == nil {
			return out, errors.New("discovery: server did not return a tool inventory")
		}
		for _, t := range page.Tools {
			out.Tools = append(out.Tools, t.Name)
		}
		if page.NextCursor == "" {
			return out, nil
		}
		if seen[page.NextCursor] {
			return out, errors.New("discovery: server repeated a tools cursor")
		}
		seen[page.NextCursor] = true
		cursor = page.NextCursor
	}
	return out, errors.New("discovery: tool inventory exceeded page limit")
}

const help = `lm — Living Memory from your terminal

  lm login [--json]               Sign in through browser OAuth
  lm logout                       Remove local account credentials
  lm inspect --account [--json]    Inspect your default World (RC3 compatibility)
  lm create <name> --room [--json]  Create and privately save a free Room
  lm rename <room> <new-name>       Rename a saved local Room alias
  lm list [--json]                 Show local Rooms and signed-in Worlds
  lm inspect [selector] [--json]   Inspect a Space; or read a Room grant from stdin
  lm remember <name> [--json]      Store memory text from stdin
  lm recall <name> [--json]        Search using query text from stdin
  lm handoff <name> [--json]       Save a temporary note from stdin
  lm resume <name> [--json]        Read the latest handoff in a new process
  lm state <name> [--json]         Read memory counts and recent memories
  lm export <name> --json          Export secret grant for another client/machine
  lm import <name> [--json]        Privately save a grant from stdin
  lm world [--json]                See the plan or activate an entitled World
  lm version

Names select a Space when unique. Use room:<alias> or world:<id> to resolve a
collision. Room grants and account credentials stay private (LM_HOME can override
the config directory). list hides door links and still shows local Rooms if the
World lookup fails. inspect shows exact entrances; a Room read may extend its
inactivity expiry. World purchase does not migrate a Room. Run lm world again
after checkout to activate access from this CLI.
`

func Run(args []string, in io.Reader, out, stderr io.Writer) int {
	return run(newClient(), args, in, out, stderr)
}
func run(c Client, args []string, in io.Reader, out, stderr io.Writer) int {
	fail := func(err error) int { fmt.Fprintln(stderr, "lm:", err); return 1 }
	if len(args) == 0 || (len(args) == 1 && (args[0] == "help" || args[0] == "--help")) {
		fmt.Fprint(out, help)
		return 0
	}
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintln(out, version)
		return 0
	}
	asJSON := false
	room := false
	useAccount := false
	var positional []string
	for _, arg := range args[1:] {
		switch arg {
		case "--json":
			asJSON = true
		case "--account":
			useAccount = true
		case "--room":
			room = true
		default:
			if strings.HasPrefix(arg, "-") {
				return fail(errors.New("unsupported option; run lm help"))
			}
			positional = append(positional, arg)
		}
	}
	if args[0] == "login" || args[0] == "logout" {
		if len(positional) != 0 || room || useAccount || (args[0] == "logout" && asJSON) {
			return fail(errors.New("usage: lm login [--json] | lm logout"))
		}
		if args[0] == "login" {
			if err := c.login(out, stderr, asJSON); err != nil {
				return fail(err)
			}
			return 0
		}
		unlock, err := c.lockAccount()
		if err != nil {
			return fail(err)
		}
		defer unlock()
		path, err := c.accountPath()
		if err != nil {
			return fail(err)
		}
		if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fail(errors.New("cannot remove local account"))
		}
		fmt.Fprintln(out, "Local account removed. Provider authorization is not revoked; revoke it in your account settings if needed.")
		return 0
	}
	selectedWorldID := ""
	if !useAccount && spaceCommand(args[0]) && len(positional) == 1 {
		alias, id, world, err := c.resolveSelector(positional[0])
		if err != nil {
			return fail(err)
		}
		if world {
			useAccount, selectedWorldID, positional = true, id, nil
		} else {
			positional[0] = alias
		}
	}
	if useAccount {
		allowed := map[string]bool{"inspect": true, "remember": true, "recall": true, "handoff": true, "resume": true, "state": true}
		if !allowed[args[0]] || room || len(positional) != 0 {
			return fail(errors.New("--account replaces the Room name for inspect/remember/recall/handoff/resume/state only"))
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
		if !a.signedIn() {
			return fail(errors.New("not signed in; run lm login"))
		}
		if selectedWorldID != "" {
			inv, e := c.fetchSpaces(&a)
			if e != nil {
				return fail(e)
			}
			found := false
			for _, s := range inv.Spaces {
				if shown(s.ID) == selectedWorldID {
					found = true
				}
			}
			if !found {
				return fail(errors.New("World is not accessible from this account"))
			}
		}
		c = c.withAccount(&a)
		c.accountGrant = &Grant{Name: "account", Kind: "world", RoomID: selectedWorldID, URL: accountResource}
		positional = []string{"@account"}
	}
	switch args[0] {
	case "rename":
		if room || useAccount || len(positional) != 2 {
			return fail(errors.New("usage: lm rename <room> <new-name> [--json]"))
		}
		alias, _, world, err := c.resolveSelector(positional[0])
		if err != nil {
			return fail(err)
		}
		if world {
			return fail(errors.New("rename supports saved local Rooms only"))
		}
		if err = c.renameRoom(alias, positional[1]); err != nil {
			return fail(err)
		}
		if asJSON {
			err = json.NewEncoder(out).Encode(map[string]string{"oldAlias": alias, "alias": positional[1], "next": "lm inspect room:" + positional[1]})
		} else {
			_, err = fmt.Fprintf(out, "Room alias renamed: %s → %s\nNext: lm inspect room:%s\n", terminalText(alias), terminalText(positional[1]), terminalText(positional[1]))
		}
		if err != nil {
			return fail(errors.New("Room alias renamed but output failed"))
		}
	case "create":
		if !room || len(positional) != 1 || strings.TrimSpace(positional[0]) == "" || len(positional[0]) > 128 || strings.ContainsAny(positional[0], "\r\n\x1b") {
			return fail(errors.New("usage: lm create <name> --room [--json] (name: 1–128 bytes)"))
		}
		path, err := c.reserve(positional[0])
		if err != nil {
			return fail(err)
		}
		g, err := c.Create(positional[0])
		if err != nil {
			return fail(err)
		}
		if err = c.save(path, g); err != nil {
			return fail(fmt.Errorf("room created but private storage failed; do not recreate: %w", err))
		}
		if asJSON {
			err = json.NewEncoder(out).Encode(summary(g))
		} else {
			err = renderSummary(out, summary(g), false)
		}
		if err != nil {
			return fail(errors.New("room created but output failed; do not blindly retry"))
		}
	case "inspect":
		if room || len(positional) > 1 {
			return fail(errors.New("usage: lm inspect [name] [--json]"))
		}
		var g Grant
		var err error
		if len(positional) == 1 {
			g, err = c.load(positional[0])
		} else {
			data, readErr := io.ReadAll(io.LimitReader(in, maxResponse+1))
			if readErr != nil || len(data) > maxResponse || json.Unmarshal(data, &g) != nil {
				err = errors.New("stdin must contain one room grant JSON")
			}
		}
		if err != nil {
			return fail(err)
		}
		info, inspectErr := c.Inspect(g)
		if len(positional) == 0 {
			info.Saved = false
		}
		if !useAccount && len(positional) == 1 && info.Identity.Source == "live" && info.Identity.RoomID != nil && *info.Identity.RoomID != g.RoomID {
			g.RoomID = *info.Identity.RoomID
			g.Warning = addresses(g.URL, g.ReadOnlyURL).Warning
			path, saveErr := c.storePath(g.Name)
			if saveErr == nil {
				saveErr = c.save(path, g)
			}
			if saveErr != nil {
				info.addIssue("local_cache", saveErr)
				inspectErr = info.failure()
			}
		}
		if asJSON {
			err = json.NewEncoder(out).Encode(info)
		} else {
			err = renderInspection(out, info)
		}
		if err != nil {
			return fail(errors.New("output failed"))
		}
		if inspectErr != nil {
			return fail(inspectErr)
		}

	default:
		return c.extra(args[0], positional, room, asJSON, in, out, stderr)
	}
	return 0
}
