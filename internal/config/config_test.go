package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	want := Config{
		APIKey:     "ht_abcdef0123456789",
		DeviceID:   "aaaabbbb-cccc-dddd-eeee-ffffffffffff",
		DeviceName: "brent-mbp",
	}
	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != want {
		t.Errorf("round-trip mismatch:\nwant %+v\ngot  %+v", want, got)
	}
}

func TestSaveEnforces0600(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if err := Save(Config{APIKey: "ht_xxx"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(filepath.Join(tmp, ".config", "ologi", "config.toml"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("got perm %o, want 0600", info.Mode().Perm())
	}
}

func TestLoadMissingFileReturnsErrNotFound(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	_, err := Load()
	if !os.IsNotExist(err) {
		t.Errorf("want os.IsNotExist(err), got %v", err)
	}
}

func TestRemoveDeletesTheFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if err := Save(Config{APIKey: "ht_xxx"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	_, err := Load()
	if !os.IsNotExist(err) {
		t.Errorf("after Remove: want not-exist, got %v", err)
	}
}
