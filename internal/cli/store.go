package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
)

var aliasPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func (c Client) storePath(name string) (string, error) {
	if !aliasPattern.MatchString(name) {
		return "", errors.New("name must be 1–64 letters, digits, underscores or hyphens, starting with a letter or digit")
	}
	if c.Home == "" {
		return "", errors.New("credential directory unavailable")
	}
	if err := os.MkdirAll(c.Home, 0700); err != nil {
		return "", errors.New("cannot create private credential directory")
	}
	info, err := os.Lstat(c.Home)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("credential directory must be a real directory accessible only to its owner (0700)")
	}
	return filepath.Join(c.Home, name+".grant.json"), nil
}

// Reserve before networking so duplicate/concurrent invocations cannot mint twice.
// An uncertain creation keeps the reservation: automatic retry is unsafe.
func (c Client) reserve(name string) (string, error) {
	path, err := c.storePath(name)
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", errors.New("name already exists or cannot be reserved; use list/inspect, do not recreate it")
	}
	_, err = f.WriteString(`{"state":"creation_pending"}` + "\n")
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return "", errors.New("could not persist creation reservation")
	}
	return path, nil
}

func (c Client) save(path string, g Grant) error {
	data, err := json.Marshal(g)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(c.Home, ".grant-*")
	if err != nil {
		return errors.New("cannot save credential")
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(data, '\n')); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return errors.New("cannot persist credential")
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return errors.New("cannot finalize credential")
	}
	return nil
}

func (c Client) load(name string) (Grant, error) {
	var g Grant
	path, err := c.storePath(name)
	if err != nil {
		return g, err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > maxResponse {
		return g, errors.New("grant missing or not a private regular file (0600)")
	}
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &g) != nil || !validAddress(g.URL) {
		return g, errors.New("grant unavailable; a previous creation may have an unknown outcome — do not blindly recreate")
	}
	g.Name = name
	if !roomIDPattern.MatchString(g.RoomID) {
		g.RoomID = ""
	}
	g.Warning = addresses(g.URL, g.ReadOnlyURL).Warning
	return g, nil
}
