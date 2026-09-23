package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

var roomIDPattern = regexp.MustCompile(`^w_[a-f0-9]{32}$`)

// Match the Theatre's supported origin and token grammar. Never send a custom
// endpoint's credential to the public Theatre, or mistake a Room ID for a door.
var theatreDoorPattern = regexp.MustCompile(`^https://lme\.viibe\.to/t/((ons|ro)_[a-f0-9]{32})/mcp/?$`)

type Identity struct {
	Alias  string  `json:"alias"`
	RoomID *string `json:"roomId"`
	Source string  `json:"source"`
}
type Addresses struct {
	Open    *string `json:"open"`
	Guide   *string `json:"guide"`
	MCP     string  `json:"mcp"`
	Access  string  `json:"access"`
	Warning string  `json:"warning"`
}
type Lifecycle struct {
	ExpiresAtAtCreation *string `json:"expiresAtAtCreation"`
	CurrentExpiresAt    *string `json:"currentExpiresAt"`
	Source              string  `json:"source"`
	Note                string  `json:"note"`
}
type RoomSummary struct {
	Space               Space     `json:"space"`
	Name                string    `json:"name"`
	Kind                string    `json:"kind"`
	ExpiresAtAtCreation string    `json:"expiresAtAtCreation"`
	Saved               bool      `json:"saved"`
	Next                string    `json:"next"`
	Identity            Identity  `json:"identity"`
	Addresses           Addresses `json:"addresses"`
	Lifecycle           Lifecycle `json:"lifecycle"`
	State               string    `json:"state,omitempty"`
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
func addresses(endpoint string, publicEndpoint ...string) Addresses {
	a := Addresses{MCP: endpoint, Access: "unknown", Warning: "Endpoint access is unknown; public read-only entrance unavailable. Keep MCP credentials private."}
	match := theatreDoorPattern.FindStringSubmatch(endpoint)
	if match == nil {
		return a
	}
	a.Access = "read_write"
	a.Warning = "Public read-only entrance unavailable for this grant. MCP is write-capable; keep it private."
	public := ""
	if match[2] == "ro" {
		a.Access = "read_only"
		public = endpoint
	} else if len(publicEndpoint) > 0 {
		public = publicEndpoint[0]
	}
	door := theatreDoorPattern.FindStringSubmatch(public)
	if door == nil || door[2] != "ro" {
		return a
	}
	link := "https://living-memory.app/theatre#" + door[1]
	a.Open, a.Guide = &link, &link
	a.Warning = "Open/Guide grant read-only access and can be shared. MCP retains the access shown above; keep a write-capable MCP URL private."
	return a
}

func summary(g Grant) RoomSummary {
	if g.Kind == "world" {
		next := "lm state --account"
		source := "unknown"
		if g.RoomID != "" {
			next = "lm state world:" + g.RoomID
			source = "selected"
		}
		return RoomSummary{Space: Space{ID: optional(g.RoomID), Name: "World", Type: "world", Access: "owner", Lifecycle: "subscription", State: "unknown", Endpoint: g.URL}, Name: "account", Kind: "world", Saved: true, Next: next, Identity: Identity{Alias: "account", RoomID: optional(g.RoomID), Source: source}, Addresses: Addresses{MCP: g.URL, Access: "authenticated", Warning: "OAuth account access; no shareable door is exposed."}, Lifecycle: Lifecycle{Source: "not_applicable", Note: "World subscription lifecycle is separate from free Room expiry."}}
	}
	id := g.RoomID
	if !roomIDPattern.MatchString(id) {
		id = ""
	}
	source := "unknown"
	if id != "" {
		source = "local_snapshot"
	}
	return RoomSummary{
		Space: Space{ID: optional(id), Name: g.Name, Type: "room", Access: addresses(g.URL, g.ReadOnlyURL).Access, Lifecycle: "inactivity_expiry", State: "saved", Endpoint: g.URL},
		Name:  g.Name, Kind: g.Kind, ExpiresAtAtCreation: g.ExpiresAt, Saved: true,
		Next:     "lm inspect " + g.Name,
		Identity: Identity{Alias: g.Name, RoomID: optional(id), Source: source}, Addresses: addresses(g.URL, g.ReadOnlyURL),
		Lifecycle: Lifecycle{ExpiresAtAtCreation: optional(g.ExpiresAt), Source: "local_snapshot", Note: g.Note},
	}
}
func shown(s *string) string {
	if s == nil {
		return "unknown"
	}
	return *s
}
func renderSummary(out io.Writer, s RoomSummary, inspect bool) error {
	if s.Kind == "world" {
		_, err := fmt.Fprintf(out, "%s · World · authenticated\n  World ID  %s\n  MCP       %s\n  Next      %s\n  %s\n", strings.Join(strings.Fields(terminalText(s.Space.Name)), " "), terminalText(shown(s.Identity.RoomID)), terminalText(s.Addresses.MCP), terminalText(s.Next), s.Addresses.Warning)
		return err
	}
	expires := shown(s.Lifecycle.ExpiresAtAtCreation)
	if !inspect && s.Lifecycle.ExpiresAtAtCreation != nil {
		if parsed, parseErr := time.Parse(time.RFC3339, expires); parseErr == nil {
			expires = parsed.Format("2 Jan 2006")
		}
	}
	_, err := fmt.Fprintf(out, "%s · Room · %s\n  Open      %s\n  Guide     %s\n  MCP       %s\n",
		terminalText(s.Name), s.Addresses.Access,
		terminalText(shown(s.Addresses.Open)), terminalText(shown(s.Addresses.Guide)), terminalText(s.Addresses.MCP))
	if err == nil && inspect {
		_, err = fmt.Fprintf(out, "  Room ID   %s (%s)\n", terminalText(shown(s.Identity.RoomID)), s.Identity.Source)
	}
	if err == nil {
		_, err = fmt.Fprintf(out, "  Expires   %s (at creation; current expiry unknown)\n", terminalText(expires))
	}
	if err == nil && s.State != "" {
		_, err = fmt.Fprintln(out, "  Local status:", s.State)
	}
	if err == nil {
		_, err = fmt.Fprintf(out, "  Next      %s\n  %s\n", terminalText(s.Next), terminalText(s.Addresses.Warning))
	}
	return err
}

type InspectionIssue struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
}
type InspectionState struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
}
type Capabilities struct {
	Status string   `json:"status"`
	Tools  []string `json:"tools"`
}
type Inspection struct {
	RoomSummary
	Tools        []string          `json:"tools"`
	Status       string            `json:"status"`
	State        InspectionState   `json:"state"`
	Capabilities Capabilities      `json:"capabilities"`
	Errors       []InspectionIssue `json:"errors"`
}

func (i *Inspection) addIssue(stage string, err error) {
	i.Status = "partial_failure"
	i.Errors = append(i.Errors, InspectionIssue{stage, err.Error()})
}
func (i Inspection) failure() error {
	if len(i.Errors) == 0 {
		return nil
	}
	parts := []string{}
	for _, issue := range i.Errors {
		parts = append(parts, issue.Stage+": "+issue.Message)
	}
	return errors.New(strings.Join(parts, "; "))
}

// Only full inspect calls identity/state tools. Normal operations use Discover.
func (c Client) Inspect(g Grant) (Inspection, error) {
	out := Inspection{RoomSummary: summary(g), Tools: []string{}, Status: "ok", State: InspectionState{Status: "unavailable"}, Capabilities: Capabilities{Status: "unavailable", Tools: []string{}}, Errors: []InspectionIssue{}}
	discovery, err := c.Discover(g)
	out.Tools = discovery.Tools
	if err != nil {
		stage, message, found := strings.Cut(err.Error(), ": ")
		if !found {
			stage, message = "initialize", err.Error()
		}
		out.addIssue(stage, errors.New(message))
		return out, out.failure()
	}
	out.Capabilities = Capabilities{Status: "live", Tools: discovery.Tools}
	if g.Kind == "world" {
		out.Space.State = "active"
	}
	has := func(tool string) bool {
		for _, name := range discovery.Tools {
			if name == tool {
				return true
			}
		}
		return false
	}
	if g.Kind != "world" && aliasPattern.MatchString(g.Name) {
		if has("handoff_read") {
			out.Next = "lm resume " + g.Name
		} else if has("memory_state") {
			out.Next = "lm state " + g.Name
		}
	}
	if has("world_list") {
		data, callErr := c.readTool(g.URL, "world_list", 102)
		if callErr == nil {
			var listing struct {
				Worlds []struct {
					ID      string  `json:"id"`
					Title   *string `json:"title"`
					Default bool    `json:"isDefault"`
				} `json:"worlds"`
			}
			if json.Unmarshal(data, &listing) != nil {
				callErr = errors.New("invalid identity result")
			} else {
				id, matches := "", 0
				var title *string
				for _, world := range listing.Worlds {
					if (g.Kind == "world" && g.RoomID != "" && world.ID == g.RoomID) || ((g.Kind != "world" || g.RoomID == "") && world.Default) {
						id = world.ID
						title = world.Title
						matches++
					}
				}
				if matches != 1 || !roomIDPattern.MatchString(id) {
					callErr = errors.New("server did not identify the selected canonical Space")
				} else {
					out.Identity.RoomID = &id
					out.Identity.Source = "live"
					out.Space.ID = &id
					if g.Kind == "world" && title != nil && *title != "" {
						out.Space.Name = *title
					}
				}
			}
		}
		if callErr != nil {
			out.addIssue("identity", callErr)
		}
	}
	if has("memory_state") {
		selectedID := ""
		if g.Kind == "world" {
			selectedID = g.RoomID
		}
		data, callErr := c.readTool(g.URL, "memory_state", 103, selectedID)
		if callErr == nil {
			var snapshot struct {
				Episodic    *int `json:"episodicCount"`
				Facets      *int `json:"selfFacetCount"`
				Prospective *int `json:"prospectiveCount"`
			}
			if json.Unmarshal(data, &snapshot) != nil || snapshot.Episodic == nil || snapshot.Facets == nil || snapshot.Prospective == nil || *snapshot.Episodic < 0 || *snapshot.Facets < 0 || *snapshot.Prospective < 0 {
				callErr = errors.New("invalid memory state result")
			}
		}
		if callErr != nil {
			out.State.Status = "error"
			out.addIssue("state", callErr)
		} else {
			out.State = InspectionState{Status: "live", Data: data}
		}
	} else {
		out.State.Status = "not_offered"
	}
	return out, out.failure()
}
func (c Client) readTool(endpoint, tool string, id int, worldID ...string) (json.RawMessage, error) {
	var result struct {
		Structured json.RawMessage `json:"structuredContent"`
		Content    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	arguments := map[string]any{}
	if len(worldID) > 0 && worldID[0] != "" {
		arguments["world_id"] = worldID[0]
	}
	if err := c.rpc(endpoint, "tools/call", map[string]any{"name": tool, "arguments": arguments}, id, &result); err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, errors.New("tool rejected the operation; no automatic retry")
	}
	if len(result.Structured) > 0 && string(result.Structured) != "null" {
		return result.Structured, nil
	}
	// MCP also permits the structured object as a JSON text content block.
	for _, item := range result.Content {
		if item.Type == "text" && json.Valid([]byte(item.Text)) {
			return json.RawMessage(item.Text), nil
		}
	}
	return nil, errors.New("tool did not return structured data")
}
func renderInspection(out io.Writer, i Inspection) error {
	if err := renderSummary(out, i.RoomSummary, true); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "  Inspection: %s\n  State: %s\n", i.Status, i.State.Status); err != nil {
		return err
	}
	if i.State.Status == "live" {
		var snapshot struct {
			Episodic    int `json:"episodicCount"`
			Facets      int `json:"selfFacetCount"`
			Prospective int `json:"prospectiveCount"`
		}
		_ = json.Unmarshal(i.State.Data, &snapshot)
		if _, err := fmt.Fprintf(out, "    %d memories · %d facets · %d pending\n", snapshot.Episodic, snapshot.Facets, snapshot.Prospective); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(out, "  Tools     %d available (%s); use --json for names\n", len(i.Tools), i.Capabilities.Status); err != nil {
		return err
	}
	for _, issue := range i.Errors {
		if _, err := fmt.Fprintf(out, "  Failed %s: %s\n", issue.Stage, terminalText(issue.Message)); err != nil {
			return err
		}
	}
	return nil
}
