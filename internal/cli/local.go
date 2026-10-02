package cli

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const localPackage = "@nature-labs/living-memory-mcp@0.1.3"

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type localConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	BaseURL  string `json:"baseUrl,omitempty"`
	KeyEnv   string `json:"keyEnv,omitempty"`
}

func privateLocalDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return errors.New("cannot create Local directory")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("Local directory must be a real private directory (0700)")
	}
	return nil
}
func (c Client) localPath(name string) (string, error) {
	if !filepath.IsAbs(c.Home) {
		return "", errors.New("Local requires an absolute LM_HOME path")
	}
	if !aliasPattern.MatchString(name) {
		return "", errors.New("Local name must be 1–64 letters, digits, underscores or hyphens")
	}
	if _, err := c.storePath("check"); err != nil {
		return "", err
	}
	root := filepath.Join(c.Home, "locals")
	if err := privateLocalDir(root); err != nil {
		return "", err
	}
	return filepath.Join(root, name), nil
}
func (c Client) readLocal(name string) (localConfig, string, error) {
	var config localConfig
	path, err := c.localPath(name)
	if err != nil {
		return config, "", err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return config, path, errors.New("Local is missing or not private; run lm list")
	}
	file := filepath.Join(path, "config.json")
	info, err = os.Lstat(file)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16384 {
		return config, path, errors.New("Local config is missing or unsafe; inspect the saved config")
	}
	data, err := os.ReadFile(file)
	if err != nil || json.Unmarshal(data, &config) != nil || config.Name != name || config.ID == "" {
		return config, path, errors.New("invalid Local config; it was not replaced")
	}
	if err = validateLocal(config); err != nil {
		return config, path, err
	}
	return config, path, nil
}
func validateLocal(config localConfig) error {
	if config.Provider == "lexical" {
		if config.Model != "unicode-fnv1a-256-v1" || config.BaseURL != "" || config.KeyEnv != "" {
			return errors.New("invalid lexical configuration")
		}
		return nil
	}
	if !map[string]bool{"ollama": true, "lmstudio": true, "openrouter": true, "openai-compatible": true}[config.Provider] || strings.TrimSpace(config.Model) == "" {
		return errors.New("choose an embedding provider and explicit model")
	}
	u, err := url.Parse(config.BaseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("embedding base URL must be HTTP(S), without credentials, query or fragment")
	}
	if config.KeyEnv != "" && !envName.MatchString(config.KeyEnv) {
		return errors.New("key-env must name an environment variable, never an API key")
	}
	if config.KeyEnv == "" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1" {
		return errors.New("network providers require --key-env VARIABLE; use a placeholder variable for an unauthenticated LAN server")
	}
	return nil
}
func saveLocal(path string, config localConfig) error {
	data, _ := json.MarshalIndent(config, "", "  ")
	file, err := os.CreateTemp(path, ".config-*")
	if err != nil {
		return errors.New("cannot save Local configuration")
	}
	defer os.Remove(file.Name())
	_, err = file.Write(append(data, '\n'))
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return errors.New("cannot persist Local configuration")
	}
	return os.Rename(file.Name(), filepath.Join(path, "config.json"))
}
func localNetwork(config localConfig) string {
	if config.Provider == "lexical" {
		return "none"
	}
	u, _ := url.Parse(config.BaseURL)
	if u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1" {
		return "loopback"
	}
	return "external (text is sent to the embedding provider)"
}
func (c Client) localServerPath() string {
	if c.localServer != "" {
		return c.localServer
	}
	if override := os.Getenv("LM_LOCAL_SERVER"); filepath.IsAbs(override) {
		return override
	}
	return filepath.Join(c.Home, "runtime", "node_modules", "@nature-labs", "living-memory-mcp", "dist", "server.js")
}
func (c Client) localEnv(config localConfig, path string) ([]string, error) {
	// Managed Locals never read the package or global legacy .env files.
	env := []string{}
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "LME_") {
			env = append(env, v)
		}
	}
	env = append(env, "LME_CONFIG_ISOLATED=1", "LME_SNAPSHOT="+filepath.Join(path, "brain.json"))
	if config.Provider == "lexical" {
		return append(env, "LME_EMBED=lexical"), nil
	}
	key := "local"
	if config.KeyEnv != "" {
		var err error
		key, _, err = c.localKey(config.KeyEnv)
		if err != nil {
			return nil, err
		}
		if key == "" {
			return nil, fmt.Errorf("embedding key is missing; run lm config keys --key-env %s, then lm doctor local:%s --probe", config.KeyEnv, config.Name)
		}
	}
	return append(env, "LME_EMBED=real", "LME_API_KEY="+key, "LME_BASE_URL="+config.BaseURL, "LME_EMBED_MODEL="+config.Model), nil
}

type localRPC struct {
	stdin     io.WriteCloser
	cmd       *exec.Cmd
	responses chan []byte
	cancel    context.CancelFunc
	ctx       context.Context
	id        int
}

func checkLocalNode() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, "node", "--version").Output()
	if err != nil {
		return errors.New("cannot verify Node version; Local requires Node >=20.12")
	}
	value := strings.TrimSpace(string(data))
	if !regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(value) {
		return errors.New("unrecognized Node version; Local requires Node >=20.12")
	}
	parts := strings.Split(strings.TrimPrefix(value, "v"), ".")
	major, e1 := strconv.Atoi(parts[0])
	minor, e2 := strconv.Atoi(parts[1])
	if e1 != nil || e2 != nil || major < 20 || (major == 20 && minor < 12) {
		return errors.New("Local requires Node >=20.12; upgrade Node before setup or Local operations")
	}
	return nil
}

func (c Client) startLocal(config localConfig, path string) (*localRPC, error) {
	if err := checkLocalNode(); err != nil {
		return nil, err
	}
	info, err := os.Stat(c.localServerPath())
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("Local runtime is missing; run lm setup local")
	}
	env, err := c.localEnv(config, path)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	cmd := exec.CommandContext(ctx, "node", c.localServerPath())
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, errors.New("cannot open Local runtime input")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, errors.New("cannot open Local runtime output")
	}
	// Raw runtime stderr can include provider details; do not echo it to terminal/JSON.
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		cancel()
		return nil, errors.New("cannot start Local runtime; run lm doctor")
	}
	rpc := &localRPC{stdin: stdin, cmd: cmd, responses: make(chan []byte), ctx: ctx, cancel: cancel}
	go func() {
		defer close(rpc.responses)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), maxResponse)
		for scanner.Scan() {
			data := append([]byte{}, scanner.Bytes()...)
			select {
			case rpc.responses <- data:
			case <-ctx.Done():
				return
			}
		}
	}()
	var result struct {
		ServerInfo struct {
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err = rpc.call("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "lm-cli", "version": version}}, &result); err != nil {
		rpc.close()
		return nil, err
	}
	if result.ServerInfo.Version != "0.1.3" {
		rpc.close()
		return nil, errors.New("Local runtime version differs; run lm setup local to install 0.1.3")
	}
	notice, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	if _, err = stdin.Write(append(notice, '\n')); err != nil {
		rpc.close()
		return nil, errors.New("Local initialization failed")
	}
	return rpc, nil
}
func (r *localRPC) close() { r.stdin.Close(); r.cancel(); r.cmd.Wait() }
func (r *localRPC) call(method string, params any, result any) error {
	r.id++
	id := r.id
	data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if _, err := r.stdin.Write(append(data, '\n')); err != nil {
		return errors.New("Local runtime input failed; outcome unknown, no automatic retry")
	}
	for {
		select {
		case <-r.ctx.Done():
			return errors.New("Local operation timed out; outcome unknown. Inspect state and lock before retrying")
		case data, ok := <-r.responses:
			if !ok {
				return errors.New("Local runtime exited or returned invalid output; check Node >=20.12 and run lm doctor")
			}
			var response struct {
				JSONRPC string          `json:"jsonrpc"`
				ID      int             `json:"id"`
				Result  json.RawMessage `json:"result"`
				Error   json.RawMessage `json:"error"`
			}
			if json.Unmarshal(data, &response) != nil || response.JSONRPC != "2.0" {
				return errors.New("invalid Local MCP response")
			}
			if response.ID != id {
				continue
			}
			if len(response.Error) > 0 {
				return errors.New("Local MCP request rejected")
			}
			if len(response.Result) == 0 || json.Unmarshal(response.Result, result) != nil {
				return errors.New("invalid Local MCP result")
			}
			return nil
		}
	}
}
func (r *localRPC) tools() ([]string, error) {
	tools := []string{}
	cursor := ""
	seen := map[string]bool{}
	for page := 0; page < 100; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
			Next string `json:"nextCursor"`
		}
		if err := r.call("tools/list", params, &result); err != nil {
			return nil, err
		}
		if result.Tools == nil {
			return nil, errors.New("Local runtime did not return tools")
		}
		for _, t := range result.Tools {
			tools = append(tools, t.Name)
		}
		if result.Next == "" {
			return tools, nil
		}
		if seen[result.Next] {
			return nil, errors.New("Local discovery cursor repeated")
		}
		seen[result.Next] = true
		cursor = result.Next
	}
	return nil, errors.New("Local discovery exceeded page limit")
}
func (r *localRPC) tool(name string, args any) (json.RawMessage, []string, error) {
	tools, err := r.tools()
	if err != nil {
		return nil, nil, err
	}
	found := false
	for _, t := range tools {
		if t == name {
			found = true
		}
	}
	if !found {
		return nil, tools, errors.New("Local runtime does not offer this tool; check its version with lm doctor")
	}
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Structured json.RawMessage `json:"structuredContent"`
		IsError    bool            `json:"isError"`
	}
	if err = r.call("tools/call", map[string]any{"name": name, "arguments": args}, &result); err != nil {
		return nil, tools, err
	}
	if result.IsError {
		message := "Local tool rejected operation; no automatic retry"
		// Managed runtime error text is controlled, but redact secret-bearing strings.
		if len(result.Content) > 0 {
			message = terminalText(result.Content[0].Text)
		}
		if len(message) > 1024 {
			message = "Local tool rejected operation; inspect storage/provider configuration"
		}
		return nil, tools, errors.New(message)
	}
	if len(result.Structured) > 0 {
		return result.Structured, tools, nil
	}
	text := []string{}
	for _, item := range result.Content {
		text = append(text, item.Text)
	}
	data, _ := json.Marshal(map[string]any{"text": strings.Join(text, "\n")})
	return data, tools, nil
}

// Handle Local before account selector resolution: local:<name> never calls OAuth.
func (c Client) runLocal(args []string, in io.Reader, out, stderr io.Writer) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	command := args[0]
	for _, arg := range args[1:] {
		if arg == "--local" && command != "create" {
			fmt.Fprintln(stderr, "lm: --local is only valid for create")
			return 1, true
		}
	}
	local := false
	for _, arg := range args[1:] {
		if arg == "--local" || strings.HasPrefix(arg, "local:") {
			local = true
		}
	}
	if command == "setup" || command == "config" || command == "doctor" || command == "mcp" || command == "serve" || command == "models" {
		local = true
	}
	candidate := ""
	for _, arg := range args[1:] {
		if !strings.HasPrefix(arg, "-") {
			candidate = arg
			break
		}
	}
	if !local && spaceCommand(command) && aliasPattern.MatchString(candidate) {
		path := filepath.Join(c.Home, "locals", candidate, "config.json")
		if _, err := os.Lstat(path); err == nil {
			local = true
			fail := false
			if _, err := os.Lstat(filepath.Join(c.Home, candidate+".grant.json")); err == nil {
				fail = true
			}
			inv, status := c.accountSpacesIfSaved()
			if status != "ok" && status != "signed_out" {
				fail = true
			}
			for _, s := range inv.Spaces {
				if strings.EqualFold(s.Name, candidate) {
					fail = true
				}
			}
			if fail {
				fmt.Fprintln(stderr, "lm: Local name is ambiguous or account inventory unavailable; use local:"+candidate)
				return 1, true
			}
		}
	}
	if !local {
		return 0, false
	}
	in = &terminalReader{reader: bufio.NewReader(in), file: inputFile(in)}
	asJSON, probe := false, false
	for _, arg := range args[1:] {
		if arg == "--json" {
			asJSON = true
		}
	}
	fail := func(err error) (int, bool) {
		fmt.Fprintln(stderr, "lm:", err)
		if asJSON {
			_ = json.NewEncoder(out).Encode(map[string]any{"status": "error", "code": "local_operation_failed", "message": err.Error(), "ready": false})
		}
		return 1, true
	}
	positions := []string{}
	options := map[string]string{}
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--json":
			asJSON = true
		case "--local":
		case "--probe":
			probe = true
		case "--lexical":
			options["provider"] = "lexical"
		case "--provider", "--model", "--base-url", "--key-env":
			if i+1 >= len(args) {
				return fail(errors.New("option requires a value"))
			}
			i++
			options[strings.TrimPrefix(arg, "--")] = args[i]
		default:
			if strings.HasPrefix(arg, "-") {
				return fail(errors.New("unsupported Local option; run lm help"))
			}
			positions = append(positions, arg)
		}
	}
	emit := func(value any) (int, bool) {
		if err := json.NewEncoder(out).Encode(value); err != nil {
			return fail(errors.New("output failed"))
		}
		return 0, true
	}
	if command == "models" {
		if len(positions) != 0 || probe || (options["provider"] != "" && options["provider"] != "openrouter") {
			return fail(errors.New("usage: lm models --provider openrouter [--key-env VARIABLE] [--json]"))
		}
		for key := range options {
			if key != "provider" && key != "key-env" {
				return fail(errors.New("unsupported model catalog option"))
			}
		}
		keyEnv := options["key-env"]
		if keyEnv == "" {
			keyEnv = "OPENROUTER_API_KEY"
		}
		if !envName.MatchString(keyEnv) {
			return fail(errors.New("invalid key variable name"))
		}
		models, err := c.embeddingModels(keyEnv)
		if err != nil {
			return fail(err)
		}
		if asJSON {
			return emit(map[string]any{"provider": "openrouter", "source": openRouterEmbeddingModels, "models": models})
		}
		renderModels(out, models)
		return 0, true
	}
	if command == "config" && len(positions) == 1 && positions[0] == "keys" {
		if probe || len(options) > 1 || (len(options) == 1 && options["key-env"] == "") {
			return fail(errors.New("usage: lm config keys [--key-env VARIABLE] [--json]"))
		}
		keyName := options["key-env"]
		if keyName == "" {
			keyName = "OPENROUTER_API_KEY"
		}
		path, err := c.configureKey(keyName, in, stderr)
		if err != nil {
			return fail(err)
		}
		if asJSON {
			return emit(map[string]any{"path": path, "variable": keyName, "configured": true})
		}
		fmt.Fprintln(out, "Key saved privately. Next: create a Local with --provider openrouter and an embedding model.")
		return 0, true
	}
	if command == "setup" {
		if len(positions) != 1 || positions[0] != "local" || len(options) != 0 || probe {
			return fail(errors.New("usage: lm setup local [--json]"))
		}
		if err := checkLocalNode(); err != nil {
			return fail(err)
		}
		runtime := filepath.Join(c.Home, "runtime")
		if _, err := c.storePath("check"); err != nil {
			return fail(err)
		}
		if err := privateLocalDir(runtime); err != nil {
			return fail(err)
		}
		data, _ := os.ReadFile(filepath.Join(runtime, "node_modules", "@nature-labs", "living-memory-mcp", "package.json"))
		var installed struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		info, statErr := os.Stat(filepath.Join(runtime, "node_modules", "@nature-labs", "living-memory-mcp", "dist", "server.js"))
		if json.Unmarshal(data, &installed) == nil && installed.Name == "@nature-labs/living-memory-mcp" && installed.Version == "0.1.3" && statErr == nil && info.Mode().IsRegular() {
			if asJSON {
				return emit(map[string]string{"package": localPackage, "status": "already_installed"})
			}
			fmt.Fprintln(out, "Local runtime is installed. Next: lm create my-memory --local")
			return 0, true
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "npm", "install", "--prefix", runtime, "--ignore-scripts", "--no-audit", "--no-fund", localPackage)
		cmd.Env = append(os.Environ(), "npm_config_cache="+filepath.Join(runtime, ".npm-cache"))
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			return fail(errors.New("Local runtime installation failed; check Node/npm and registry access. No Local memories were changed"))
		}
		if asJSON {
			return emit(map[string]string{"package": localPackage, "status": "installed"})
		}
		fmt.Fprintln(out, "Local runtime installed. Next: lm create my-memory --local")
		return 0, true
	}
	if len(positions) != 1 {
		return fail(errors.New("Local command requires one name: lm <command> local:<name> [--json]"))
	}
	name := strings.TrimPrefix(positions[0], "local:")
	if !aliasPattern.MatchString(name) {
		return fail(errors.New("invalid Local name"))
	}
	if probe && command != "doctor" {
		return fail(errors.New("--probe is only valid for doctor"))
	}
	if len(options) > 0 && command != "create" && command != "config" {
		return fail(errors.New("provider options are only valid for create/config"))
	}
	var config localConfig
	var path string
	var err error
	if command == "create" {
		path, err = c.localPath(name)
		if err != nil {
			return fail(err)
		}
		config = localConfig{Name: name, Provider: "lexical", Model: "unicode-fnv1a-256-v1"}
		if !asJSON && interactiveInput(in) && options["provider"] == "" {
			fmt.Fprint(stderr, "Embedding: [1] OpenRouter  [2] Ollama  [3] LM Studio  [4] Lexical (no model/network)\nChoose [1]: ")
			line, e := inputLine(in)
			if e != nil && e != io.EOF {
				return fail(errors.New("cannot read provider choice"))
			}
			provider := map[string]string{"": "openrouter", "1": "openrouter", "2": "ollama", "3": "lmstudio", "4": "lexical"}[strings.TrimSpace(line)]
			if provider == "" {
				return fail(errors.New("choose provider 1–4 or specify --provider explicitly"))
			}
			options["provider"] = provider
		}

		id := make([]byte, 16)
		if _, err = rand.Read(id); err != nil {
			return fail(errors.New("cannot create Local identity"))
		}
		config.ID = "local_" + hex.EncodeToString(id)
	} else {
		config, path, err = c.readLocal(name)
		if err != nil {
			return fail(err)
		}
	}
	if command == "config" && len(options) > 0 {
		// Share the runtime's brain.json operation lock across check and save.
		lock := filepath.Join(path, "brain.json.lock")
		if err := os.Mkdir(lock, 0700); err != nil {
			return fail(errors.New("Local store is busy or locked; config was not changed. Inspect the operation before retrying"))
		}
		defer os.RemoveAll(lock)
		owner, _ := json.Marshal(map[string]any{"pid": os.Getpid(), "createdAt": time.Now().UTC().Format(time.RFC3339Nano)})
		if err := os.WriteFile(filepath.Join(lock, "owner.json"), owner, 0600); err != nil {
			return fail(errors.New("cannot persist Local operation lock; config was not changed"))
		}
		// Another configuration writer may have completed before we acquired the lock.
		config, path, err = c.readLocal(name)
		if err != nil {
			return fail(err)
		}
	}
	if command == "create" || command == "config" {
		if provider, ok := options["provider"]; ok {
			config.Provider = provider
			config.BaseURL = map[string]string{"ollama": "http://127.0.0.1:11434/v1", "lmstudio": "http://127.0.0.1:1234/v1", "openrouter": "https://openrouter.ai/api/v1"}[provider]
			config.KeyEnv = ""
			config.Model = ""
			if provider == "lexical" {
				config.Model = "unicode-fnv1a-256-v1"
			}
			if provider == "openrouter" {
				config.KeyEnv = "OPENROUTER_API_KEY"
			}
		}
		if v, ok := options["model"]; ok {
			config.Model = v
		}
		if v, ok := options["base-url"]; ok {
			config.BaseURL = strings.TrimSuffix(v, "/")
		}
		if v, ok := options["key-env"]; ok {
			config.KeyEnv = v
		}
		if config.Provider == "openrouter" && config.Model == "" {
			if asJSON || !interactiveInput(in) {
				return fail(errors.New("choose an embedding model with lm models --provider openrouter --json, then pass --model ID; interactive creation offers a live picker"))
			}
			key, _, e := c.localKey(config.KeyEnv)
			if e != nil {
				return fail(e)
			}
			if key == "" {
				if _, e = c.configureKey(config.KeyEnv, in, stderr); e != nil {
					return fail(e)
				}
			}
			config.Model, err = c.chooseEmbeddingModel(config.KeyEnv, in, stderr)
			if err != nil {
				return fail(err)
			}
		}
		if err = validateLocal(config); err != nil {
			return fail(err)
		}
		if command == "create" {
			if err = os.Mkdir(path, 0700); err != nil {
				return fail(errors.New("Local already exists or cannot be created; run lm list"))
			}
		} else if len(options) > 0 {
			// Provider identity is immutable once vectors exist. Compare the store metadata,
			// not credentials, before committing a configuration change.
			data, readErr := os.ReadFile(filepath.Join(path, "brain.json"))
			if readErr == nil {
				var saved struct {
					Identity *struct {
						Mode     string  `json:"mode"`
						Model    string  `json:"model"`
						Endpoint *string `json:"endpoint"`
					} `json:"localEmbedding"`
					Episodic    []any `json:"episodic"`
					SelfFacets  []any `json:"selfFacets"`
					Prospective []any `json:"prospective"`
					Persons     map[string]struct {
						Episodic []any `json:"episodic"`
					} `json:"persons"`
				}
				if json.Unmarshal(data, &saved) != nil {
					return fail(errors.New("store is corrupt; config was not changed"))
				}
				mode, model, endpoint := "semantic", config.Model, config.BaseURL
				if config.Provider == "lexical" {
					mode = "lexical"
					endpoint = ""
				}
				populated := len(saved.Episodic) > 0 || len(saved.SelfFacets) > 0 || len(saved.Prospective) > 0
				for _, person := range saved.Persons {
					populated = populated || len(person.Episodic) > 0
				}
				if populated && (saved.Identity == nil || saved.Identity.Mode != mode || saved.Identity.Model != model || (saved.Identity.Endpoint != nil && *saved.Identity.Endpoint != endpoint)) {
					return fail(errors.New("embedding identity is bound to this Local; create another Local instead of changing its model"))
				}
			} else if !os.IsNotExist(readErr) {
				return fail(errors.New("cannot inspect store; config was not changed"))
			}
		}
		if command == "create" || len(options) > 0 {
			if err = saveLocal(path, config); err != nil {
				return fail(err)
			}
		}
		if asJSON {
			return emit(config)
		}
		next := "lm doctor local:" + name
		if config.Provider != "lexical" {
			next += " --probe"
		}
		fmt.Fprintf(out, "Local %s\n  ID: %s\n  Storage: %s\n  Embedding: %s / %s\n  Network: %s\nNext: %s\n", name, config.ID, filepath.Join(path, "brain.json"), config.Provider, terminalText(config.Model), localNetwork(config), next)
		return 0, true
	}
	if command == "mcp" {
		executable, err := os.Executable()
		if err != nil {
			return fail(errors.New("cannot locate lm executable"))
		}
		return emit(map[string]any{"mcpServers": map[string]any{name: map[string]any{"command": executable, "args": []string{"serve", "local:" + name}, "env": map[string]string{"LM_HOME": c.Home}}}})
	}
	if command == "serve" {
		if err := checkLocalNode(); err != nil {
			return fail(err)
		}
		env, err := c.localEnv(config, path)
		if err != nil {
			return fail(err)
		}
		cmd := exec.Command("node", c.localServerPath())
		cmd.Env = env
		cmd.Stdin = in
		cmd.Stdout = out
		cmd.Stderr = stderr
		if err = cmd.Run(); err != nil {
			return fail(errors.New("Local MCP server exited; run lm doctor local:" + name))
		}
		return 0, true
	}
	tool := map[string]string{"remember": "memory_add", "recall": "memory_search", "state": "memory_state", "handoff": "handoff_post", "resume": "handoff_read", "inspect": "local_info", "doctor": "local_info"}[command]
	if tool == "" {
		return fail(errors.New("unsupported Local command; grant export/import do not transfer Local memories"))
	}
	params := map[string]any{}
	if tool == "local_info" {
		params["probe"] = probe
	}
	if command == "remember" || command == "recall" || command == "handoff" {
		data, e := io.ReadAll(io.LimitReader(in, 65537))
		if e != nil || len(data) > 65536 || strings.TrimSpace(string(data)) == "" {
			return fail(errors.New("stdin must contain 1–65536 bytes of text"))
		}
		key := map[string]string{"remember": "content", "recall": "query", "handoff": "text"}[command]
		params[key] = string(data)
		if command == "handoff" {
			params["from"] = "lm-cli"
		}
	}
	rpc, err := c.startLocal(config, path)
	if err != nil && (command == "inspect" || command == "doctor") {
		next := "lm setup local"
		if strings.Contains(err.Error(), "embedding key is missing") {
			next = "lm config keys --key-env " + config.KeyEnv
		}
		info := map[string]any{"id": config.ID, "name": name, "configuration": "local", "storage": filepath.Join(path, "brain.json"), "embedding": config, "network": localNetwork(config), "door": nil, "inference": "not_used", "ready": false, "issue": err.Error(), "next": next}
		if asJSON {
			emit(info)
		} else {
			fmt.Fprintf(out, "Local %s\n  Storage: %s\n  Embedding: %s / %s\n  Network: %s\n  Ready: false\n  Issue: %s\n", name, info["storage"], config.Provider, config.Model, info["network"], err)
		}
		return 1, true
	}
	if err != nil {
		return fail(err)
	}
	defer rpc.close()
	data, tools, err := rpc.tool(tool, params)
	if err != nil {
		return fail(err)
	}
	var result map[string]any
	if json.Unmarshal(data, &result) != nil {
		return fail(errors.New("invalid Local structured result"))
	}
	if command == "inspect" || command == "doctor" {
		result["id"] = config.ID
		result["name"] = name
		result["tools"] = tools
		result["runtimeReady"] = true
		if config.KeyEnv != "" {
			key, source, _ := c.localKey(config.KeyEnv)
			result["credential"] = map[string]any{"variable": config.KeyEnv, "configured": key != "", "source": source}
		}
		embedding, _ := result["embedding"].(map[string]any)
		ready := config.Provider == "lexical" || probe
		issue := "semantic provider not probed; run lm doctor local:" + name + " --probe (generic request, may incur provider charges)"
		if ready {
			issue = ""
		}
		if compatible, ok := embedding["compatible"].(bool); ok && !compatible {
			ready = false
			issue = "embedding configuration differs from this store; use its original configuration"
		}
		if legacy, ok := embedding["legacy"].(bool); ok && legacy {
			ready = false
			issue = "legacy embedding identity requires explicit adoption"
		}
		result["ready"] = ready
		result["issue"] = issue
		if asJSON {
			emit(result)
		} else {
			dimensions := embedding["dimensions"]
			if dimensions == nil {
				dimensions = "unknown"
			}
			endpoint := config.BaseURL
			if endpoint == "" {
				endpoint = "none"
			}
			fmt.Fprintf(out, "Local %s\n  ID: %s\n  Storage: %s\n  Embedding: %s / %s\n  Endpoint: %s\n  Dimensions: %v\n  Network: %s\n  Inference: not used\n  Door: none\n  Runtime ready: true\n  Ready: %t\n", name, config.ID, filepath.Join(path, "brain.json"), config.Provider, terminalText(config.Model), terminalText(endpoint), dimensions, localNetwork(config), ready)
			if credential, ok := result["credential"].(map[string]any); ok {
				fmt.Fprintf(out, "  Credential: configured=%v, source=%v\n", credential["configured"], credential["source"])
			}
			if issue != "" {
				fmt.Fprintln(out, "  "+issue)
			}
		}
		if command == "doctor" && !ready {
			return 1, true
		}
		return 0, true
	}
	if asJSON {
		return emit(result)
	}
	text, _ := result["text"].(string)
	if text == "" && command == "remember" {
		text = "Remembered locally."
	}
	if text == "" && command == "handoff" {
		text = fmt.Sprintf("Handoff saved: %v (expires %v)", result["id"], result["expiresAt"])
	}
	if text == "" && command == "resume" {
		text = "No live handoff."
	}
	fmt.Fprintln(out, terminalText(text))
	return 0, true
}

func (c Client) localSpaces() ([]Space, error) {
	rows := []Space{}
	root := filepath.Join(c.Home, "locals")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return rows, nil
	}
	if err != nil {
		return nil, errors.New("cannot read Local inventory")
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		config, _, err := c.readLocal(entry.Name())
		state := "configured"
		var id *string
		if err != nil {
			state = "needs_attention"
		} else {
			id = &config.ID
		}
		rows = append(rows, Space{ID: id, Name: entry.Name(), Type: "local", Access: "device_owner", Lifecycle: "device_storage", State: state})
	}
	return rows, nil
}
