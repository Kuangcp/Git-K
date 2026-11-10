// Package settings stores the app's configuration as JSON, with defaults
// for what a file lacks. Add a field to Config for each new setting.
package settings

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Version is the schema's version, written to the file.
const Version = 1

// Config is the app's persisted configuration.
type Config struct {
	Version int      `json:"version"`
	Layout  Layout   `json:"layout"`
	Window  Window   `json:"window"`
	Recent  []string `json:"recent,omitempty"`
}

// Window is the main window's last position and size, in DIPs.
type Window struct {
	X, Y, Width, Height int
	// Positioned says X and Y were saved, so a centered window is not
	// mistaken for one at the screen's origin.
	Positioned bool `json:"positioned,omitempty"`
}

// Layout holds the window's split ratios, as percentages.
type Layout struct {
	SidePercent   float32 `json:"side_percent"`
	CommitPercent float32 `json:"commit_percent"`
}

// Default is the configuration used for a new file and for fields that a
// file does not set.
func Default() Config {
	return Config{
		Version: Version,
		Layout: Layout{
			SidePercent:   48,
			CommitPercent: 44,
		},
		Window: Window{Width: 1280, Height: 820},
	}
}

// Store reads and writes a Config, saving updates after a short delay so
// dragging a split does not write a file on every frame.
type Store struct {
	mu    sync.Mutex
	path  string
	cfg   Config
	timer *time.Timer
}

const saveDelay = 400 * time.Millisecond

// Open loads the configuration at path. A missing or malformed file
// gives the defaults, so a broken file never stops the app. An empty
// path keeps the store in memory.
func Open(path string) (*Store, error) {
	s := &Store{path: path, cfg: Default()}
	if path == "" {
		return s, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return s, nil
	}
	s.cfg = withDefaults(cfg)
	return s, nil
}

// Config returns a copy of the current configuration.
func (s *Store) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// Update changes the configuration and schedules a save. It is safe from
// any goroutine.
func (s *Store) Update(fn func(*Config)) {
	if fn == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.cfg)
	s.scheduleLocked()
}

// Flush stops a pending save and writes the configuration now.
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	return s.writeLocked()
}

func (s *Store) scheduleLocked() {
	if s.path == "" {
		return
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(saveDelay, func() {
		if err := s.Flush(); err != nil {
			log.Printf("保存设置失败: %v", err)
		}
	})
}

func (s *Store) writeLocked() error {
	if s.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".settings-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.path)
}

// DefaultPath is where the configuration lives on this system, or "" when
// the system has no configuration directory.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "git-k", "settings.json")
}

func withDefaults(cfg Config) Config {
	d := Default()
	if cfg.Version == 0 {
		cfg.Version = d.Version
	}
	if cfg.Layout.SidePercent <= 0 || cfg.Layout.SidePercent >= 100 {
		cfg.Layout.SidePercent = d.Layout.SidePercent
	}
	if cfg.Layout.CommitPercent <= 0 || cfg.Layout.CommitPercent >= 100 {
		cfg.Layout.CommitPercent = d.Layout.CommitPercent
	}
	if cfg.Window.Width <= 0 {
		cfg.Window.Width = d.Window.Width
	}
	if cfg.Window.Height <= 0 {
		cfg.Window.Height = d.Window.Height
	}
	return cfg
}
