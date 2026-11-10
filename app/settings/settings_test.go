package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Config(); !reflect.DeepEqual(got, Default()) {
		t.Fatalf("defaults: %+v, want %+v", got, Default())
	}
	s.Update(func(c *Config) {
		c.Layout.SidePercent = 61
		c.Layout.CommitPercent = 37
		c.Recent = []string{"/work/b", "/work/a"}
		c.Window = Window{X: 120, Y: 80, Width: 1000, Height: 700, Positioned: true}
	})
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}

	loaded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.Config()
	if got.Layout.SidePercent != 61 || got.Layout.CommitPercent != 37 {
		t.Fatalf("loaded layout: %+v", got.Layout)
	}
	if !reflect.DeepEqual(got.Recent, []string{"/work/b", "/work/a"}) {
		t.Fatalf("loaded recent: %v", got.Recent)
	}
	if got.Window != (Window{X: 120, Y: 80, Width: 1000, Height: 700, Positioned: true}) {
		t.Fatalf("loaded window: %+v", got.Window)
	}
	if got.Version != Version {
		t.Fatalf("version: %d, want %d", got.Version, Version)
	}
}

// A missing, malformed, or partial file still gives usable settings.
func TestDefaultsFallback(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"missing.json": "",
		"broken.json":  "{not json",
		"partial.json": `{"layout":{"side_percent":70}}`,
		"zero.json":    `{"layout":{"side_percent":0,"commit_percent":0}}`,
	} {
		path := filepath.Join(dir, name)
		if body != "" {
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		s, err := Open(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := s.Config()
		if name == "partial.json" {
			if got.Layout.SidePercent != 70 || got.Layout.CommitPercent != Default().Layout.CommitPercent {
				t.Fatalf("%s: %+v", name, got.Layout)
			}
			continue
		}
		if !reflect.DeepEqual(got, Default()) {
			t.Fatalf("%s: %+v, want defaults", name, got)
		}
	}
}

// An empty path is an in-memory store that never writes.
func TestInMemory(t *testing.T) {
	s, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	s.Update(func(c *Config) { c.Layout.SidePercent = 80 })
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if got := s.Config().Layout.SidePercent; got != 80 {
		t.Fatalf("side percent: %v", got)
	}
}
