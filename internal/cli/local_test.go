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

func localTestClient(t *testing.T) Client {
	t.Helper()
	c := newClient()
	c.Home = filepath.Join(t.TempDir(), "private")
	path, err := filepath.Abs("../../../living-memory-engine/lme-mcp/dist/server.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("build local MCP before integration tests:", err)
	}
	c.localServer = path
	c.HTTP = &http.Client{Transport: localRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("explicit Local selector made an account/hosted request")
		return nil, nil
	})}
	return c
}
func localRun(t *testing.T, c Client, args []string, input string) (int, string, string) {
	t.Helper()
	var out, stderr bytes.Buffer
	code := run(c, args, strings.NewReader(input), &out, &stderr)
	return code, out.String(), stderr.String()
}
func TestLocalLoopAcrossStdioProcesses(t *testing.T) {
	c := localTestClient(t)
	for _, step := range []struct {
		args        []string
		input, want string
	}{
		{[]string{"create", "notes", "--local", "--json"}, "", "local_"},
		{[]string{"doctor", "local:notes", "--json"}, "", `"ready":true`},
		{[]string{"remember", "local:notes", "--json"}, "เชียงใหม่ has a local capsule", `"remembered":true`},
		{[]string{"recall", "local:notes", "--json"}, "เชียงใหม่", "เชียงใหม่ has a local capsule"},
		{[]string{"handoff", "local:notes", "--json"}, "next: review\nkeep exact text", `"id":"h_`},
		{[]string{"resume", "local:notes"}, "", "next: review\nkeep exact text"},
		{[]string{"state", "local:notes", "--json"}, "", `"episodic":1`},
		{[]string{"inspect", "local:notes", "--json"}, "", `"inference":"not_used"`},
		{[]string{"mcp", "local:notes"}, "", `"serve","local:notes"`},
	} {
		code, out, err := localRun(t, c, step.args, step.input)
		if code != 0 || !strings.Contains(out, step.want) {
			t.Fatalf("%v: code=%d out=%s err=%s", step.args, code, out, err)
		}
		if strings.Contains(strings.Join(step.args, " "), "--json") && !json.Valid([]byte(out)) {
			t.Fatal("invalid JSON", out)
		}
	}
	code, _, err := localRun(t, c, []string{"config", "local:notes", "--provider", "ollama", "--model", "embeddinggemma"}, "")
	if code != 1 || !strings.Contains(err, "bound to this Local") {
		t.Fatal("model change was not refused", err)
	}
	code, _, err = localRun(t, c, []string{"export", "local:notes", "--json"}, "")
	if code != 1 || !strings.Contains(err, "do not transfer") {
		t.Fatal("Local was exported as a grant", err)
	}
}
func TestLocalMixedInventoryAndCollision(t *testing.T) {
	c := localTestClient(t)
	code, _, err := localRun(t, c, []string{"create", "notes", "--local"}, "")
	if code != 0 {
		t.Fatal(err)
	}
	path, e := c.reserve("notes")
	if e != nil {
		t.Fatal(e)
	}
	if e = c.save(path, Grant{Name: "notes", Kind: "room", URL: "https://lme.viibe.to/t/ons_fake/mcp", ExpiresAt: "2026-10-03T00:00:00Z"}); e != nil {
		t.Fatal(e)
	}
	code, out, err := localRun(t, c, []string{"list", "--json"}, "")
	if code != 0 || !strings.Contains(out, `"type":"local"`) || !strings.Contains(out, `"type":"room"`) {
		t.Fatal(code, out, err)
	}
	code, _, err = localRun(t, c, []string{"state", "notes"}, "")
	if code != 1 || !strings.Contains(err, "ambiguous") {
		t.Fatal("collision was not detected", err)
	}
	code, _, err = localRun(t, c, []string{"state", "local:notes"}, "")
	if code != 0 {
		t.Fatal(err)
	}
}
func TestLocalDoctorMissingRuntimeAndRemoteKey(t *testing.T) {
	c := localTestClient(t)
	c.localServer = filepath.Join(t.TempDir(), "missing.js")
	code, _, err := localRun(t, c, []string{"create", "notes", "--local", "--provider", "openrouter", "--model", "fixture-model", "--key-env", "LM_TEST_MISSING_KEY"}, "")
	if code != 0 {
		t.Fatal(err)
	}
	code, out, _ := localRun(t, c, []string{"doctor", "local:notes", "--json"}, "")
	if code != 1 || !json.Valid([]byte(out)) || !strings.Contains(out, `"ready":false`) {
		t.Fatal(code, out)
	}
	config, path, e := c.readLocal("notes")
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("LM_TEST_MISSING_KEY", "test-secret-never-output")
	env, e := c.localEnv(config, path)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(strings.Join(env, "\n"), "LME_API_KEY=test-secret-never-output") {
		t.Fatal("key reference was not resolved")
	}
	code, out, err = localRun(t, c, []string{"config", "local:notes", "--json"}, "")
	if code != 0 || strings.Contains(out+err, "test-secret-never-output") {
		t.Fatal("secret leak", out, err)
	}
}

type localRoundTripFunc func(*http.Request) (*http.Response, error)

func (f localRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestLocalSemanticFixtureDoesNotProbeDuringInspect(t *testing.T) {
	c := localTestClient(t)
	calls := 0
	dimensions := 2
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		vector := make([]float64, dimensions)
		vector[0] = 1
		json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"embedding": vector}}})
	}))
	defer fixture.Close()
	code, _, err := localRun(t, c, []string{"create", "semantic", "--local", "--provider", "openai-compatible", "--base-url", fixture.URL + "/v1", "--model", "fixture"}, "")
	if code != 0 {
		t.Fatal(err)
	}
	code, out, err := localRun(t, c, []string{"inspect", "local:semantic", "--json"}, "")
	if code != 0 || calls != 0 || !strings.Contains(out, `"ready":false`) {
		t.Fatal("implicit probe or false readiness", code, calls, out, err)
	}
	code, out, err = localRun(t, c, []string{"doctor", "local:semantic", "--probe", "--json"}, "")
	if code != 0 || calls != 1 || !strings.Contains(out, `"dimensions":2`) {
		t.Fatal("probe", code, calls, out, err)
	}
	code, _, err = localRun(t, c, []string{"remember", "local:semantic"}, "a semantic fixture fact")
	if code != 0 {
		t.Fatal(err)
	}
	config, path, e := c.readLocal("semantic")
	if e != nil {
		t.Fatal(e)
	}
	_ = config
	before, e := os.ReadFile(filepath.Join(path, "brain.json"))
	if e != nil {
		t.Fatal(e)
	}
	dimensions = 3
	code, out, err = localRun(t, c, []string{"remember", "local:semantic", "--json"}, "must not persist")
	if code != 1 || !json.Valid([]byte(out)) || !strings.Contains(err, "dimensions changed") {
		t.Fatal(code, out, err)
	}
	after, e := os.ReadFile(filepath.Join(path, "brain.json"))
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("incompatible embedding changed the store")
	}
}

func TestPrivateKeyConfigAndLiveCatalogContract(t *testing.T) {
	c := localTestClient(t)
	t.Setenv("OPENROUTER_API_KEY", "")
	secret := "fake-key-never-print"
	code, out, err := localRun(t, c, []string{"config", "keys", "--json"}, secret+"\n")
	if code != 0 || strings.Contains(out+err, secret) {
		t.Fatal("credential setup leaked or failed", code, out, err)
	}
	info, e := os.Stat(filepath.Join(c.Home, "lm.config"))
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal("credential file is not private")
	}
	config := localConfig{Name: "notes", Provider: "openrouter", Model: "fixture", BaseURL: "https://openrouter.ai/api/v1", KeyEnv: "OPENROUTER_API_KEY"}
	env, e := c.localEnv(config, c.Home)
	if e != nil || !strings.Contains(strings.Join(env, "\n"), "LME_API_KEY="+secret) {
		t.Fatal("private credential was not used")
	}
	t.Setenv("OPENROUTER_API_KEY", "environment-wins")
	key, source, e := c.localKey("OPENROUTER_API_KEY")
	if e != nil || key != "environment-wins" || source != "environment" {
		t.Fatal("environment precedence")
	}
	t.Setenv("OPENROUTER_API_KEY", "")
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			t.Error("catalog omitted configured key")
		}
		fmt.Fprint(w, `{"data":[{"id":"vendor/second","name":"Second","context_length":2048,"pricing":{"prompt":"0.00000002"}},{"id":"vendor/first","name":"First","pricing":{"prompt":"0"}}]}`)
	}))
	defer fixture.Close()
	c.HTTP = fixture.Client()
	c.modelsURL = fixture.URL
	code, out, err = localRun(t, c, []string{"models", "--provider", "openrouter", "--json"}, "")
	if code != 0 || !json.Valid([]byte(out)) || !strings.Contains(out, "vendor/first") || strings.Contains(out+err, secret) {
		t.Fatal(code, out, err)
	}
	var catalog struct {
		Models []embeddingModel `json:"models"`
	}
	json.Unmarshal([]byte(out), &catalog)
	if len(catalog.Models) != 2 || catalog.Models[0].ID != "vendor/first" {
		t.Fatal("model IDs were not discovered")
	}
	var prompt bytes.Buffer
	selected, e := c.chooseEmbeddingModel("OPENROUTER_API_KEY", strings.NewReader("2\n"), &prompt)
	if e != nil || selected != "vendor/second" || strings.Contains(prompt.String(), secret) {
		t.Fatal("picker", selected, e)
	}
	code, _, err = localRun(t, c, []string{"create", "notes", "--local", "--provider", "openrouter", "--json"}, "")
	if code != 1 || !strings.Contains(err, "lm models") {
		t.Fatal("agent create should require an explicit model", code, err)
	}
	if e = os.Chmod(filepath.Join(c.Home, "lm.config"), 0644); e != nil {
		t.Fatal(e)
	}
	_, _, e = c.localKey("OPENROUTER_API_KEY")
	if e == nil {
		t.Fatal("unsafe credential file accepted")
	}
}

func TestLocalSetupChecksNodeBeforeInstalling(t *testing.T) {
	for _, version := range []string{"v18.20.8", "v20.11.1", "invalid", "v20.12.0", "v22.0.0"} {
		t.Run(version, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PATH", dir)
			if err := os.WriteFile(filepath.Join(dir, "node"), []byte("#!/bin/sh\nprintf '%s\\n' '"+version+"'\n"), 0700); err != nil {
				t.Fatal(err)
			}
			c := newClient()
			c.Home = filepath.Join(t.TempDir(), "home")
			valid := version == "v20.12.0" || version == "v22.0.0"
			if valid {
				entry := filepath.Join(c.Home, "runtime", "node_modules", "@nature-labs", "living-memory-mcp")
				if err := os.MkdirAll(filepath.Join(entry, "dist"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(entry, "package.json"), []byte(`{"name":"@nature-labs/living-memory-mcp","version":"0.1.3"}`), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(entry, "dist", "server.js"), []byte("// fixture"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			code, out, stderr := localRun(t, c, []string{"setup", "local", "--json"}, "")
			if valid {
				if code != 0 || !strings.Contains(out, "already_installed") {
					t.Fatal(code, out, stderr)
				}
			} else {
				if code != 1 || !strings.Contains(stderr, "Node >=20.12") || !json.Valid([]byte(out)) {
					t.Fatal(code, out, stderr)
				}
				if _, err := os.Stat(c.Home); !os.IsNotExist(err) {
					t.Fatal("setup wrote files before rejecting Node", err)
				}
			}
		})
	}
}
