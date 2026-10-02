package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

func (c Client) readKeys() (map[string]string, error) {
	keys := map[string]string{}
	if _, err := c.storePath("check"); err != nil {
		return nil, err
	}
	path := filepath.Join(c.Home, "lm.config")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return keys, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 65536 || info.Sys() == nil {
		return nil, errors.New("lm.config must be a private regular file (0600), at most 64 KiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("cannot read private lm.config")
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !ok || !envName.MatchString(key) {
			return nil, errors.New("invalid lm.config: use VARIABLE=value lines; values are never executed")
		}
		if _, exists := keys[key]; exists {
			return nil, errors.New("duplicate variable in lm.config; remove the duplicate explicitly")
		}
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		if strings.ContainsAny(value, "\r\n\x00") {
			return nil, errors.New("invalid single-line key in lm.config")
		}
		keys[key] = value
	}
	return keys, nil
}
func (c Client) localKey(name string) (string, string, error) {
	if value := os.Getenv(name); value != "" {
		return value, "environment", nil
	}
	keys, err := c.readKeys()
	if err != nil {
		return "", "", err
	}
	if value := keys[name]; value != "" {
		return value, "lm.config", nil
	}
	return "", "missing", nil
}

func (c Client) configureKey(name string, in io.Reader, prompt io.Writer) (string, error) {
	if !envName.MatchString(name) {
		return "", errors.New("key-env must name an environment variable")
	}
	if _, err := c.storePath("check"); err != nil {
		return "", err
	}
	lock := filepath.Join(c.Home, "lm.config.lock")
	if err := os.Mkdir(lock, 0700); err != nil {
		return "", errors.New("credential configuration is busy or locked; inspect the lock before retrying")
	}
	defer os.RemoveAll(lock)
	if err := os.WriteFile(filepath.Join(lock, "owner"), []byte(fmt.Sprint(os.Getpid())), 0600); err != nil {
		return "", errors.New("cannot persist credential lock")
	}
	keys, err := c.readKeys()
	if err != nil {
		return "", err
	}
	if file := inputFile(in); file != nil {
		if info, e := file.Stat(); e == nil && info.Mode()&os.ModeCharDevice != 0 {
			// Supported desktop platforms have stty; never fall back to visibly echoing a key.
			state := exec.Command("stty", "-g")
			state.Stdin = file
			previous, e := state.Output()
			if e != nil {
				return "", errors.New("cannot hide terminal input; supply the key via private stdin instead")
			}
			hide := exec.Command("stty", "-echo")
			hide.Stdin = file
			if e = hide.Run(); e != nil {
				return "", errors.New("cannot hide terminal input")
			}
			defer func() {
				restore := exec.Command("stty", strings.TrimSpace(string(previous)))
				restore.Stdin = file
				_ = restore.Run()
				fmt.Fprintln(prompt)
			}()
			fmt.Fprint(prompt, "API key (input hidden): ")
		}
	}
	var line string
	if interactiveInput(in) {
		interrupted := make(chan os.Signal, 1)
		signal.Notify(interrupted, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(interrupted)
		type inputResult struct {
			line string
			err  error
		}
		read := make(chan inputResult, 1)
		go func() { line, err := inputLine(in); read <- inputResult{line, err} }()
		select {
		case value := <-read:
			line, err = value.line, value.err
		case <-interrupted:
			return "", errors.New("key entry cancelled; nothing was saved")
		}
	} else {
		line, err = inputLine(in)
	}
	if err != nil && err != io.EOF {
		return "", errors.New("cannot read key input")
	}
	value := strings.TrimSpace(line)
	if value == "" || len(value) > 8192 || strings.ContainsAny(value, "\r\n\x00\t ") {
		return "", errors.New("provide one nonempty single-line API key through stdin")
	}
	keys[name] = value
	names := []string{}
	for key := range keys {
		names = append(names, key)
	}
	sort.Strings(names)
	var body strings.Builder
	body.WriteString("# Private CLI credentials. Environment variables override these values.\n")
	for _, key := range names {
		fmt.Fprintf(&body, "%s=%s\n", key, keys[key])
	}
	temp, err := os.CreateTemp(c.Home, ".keys-*")
	if err != nil {
		return "", errors.New("cannot save private key file")
	}
	defer os.Remove(temp.Name())
	_, err = temp.WriteString(body.String())
	closeErr := temp.Close()
	if err != nil || closeErr != nil {
		return "", errors.New("cannot persist private key file")
	}
	path := filepath.Join(c.Home, "lm.config")
	if err = os.Rename(temp.Name(), path); err != nil {
		return "", errors.New("cannot finalize private key file")
	}
	return path, nil
}
