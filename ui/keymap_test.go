package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"git-k/app/settings"
)

// Chords round-trip through the settings text form.
func TestChordParseFormat(t *testing.T) {
	cases := []string{"Ctrl+S", "Ctrl+Shift+S", "F5", "Ctrl+Enter", "Ctrl+Alt+P", "Super+/"}
	for _, want := range cases {
		cs, err := parseChords(want)
		if err != nil {
			t.Fatalf("parse %q: %v", want, err)
		}
		if got := formatChords(cs); got != want {
			t.Fatalf("format %q: got %q", want, got)
		}
	}
	cs, err := parseChords("Ctrl+Enter, F5")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 || cs[0].Key != ui.KeyEnter || cs[1].Key != ui.KeyF5 {
		t.Fatalf("parsed %+v", cs)
	}
	if _, err := parseChords("Ctrl+Nope"); err == nil {
		t.Fatal("malformed chord parsed")
	}
}

// Setting a chord unbinds it from the other actions of the same scope
// only.
func TestBindingsConflict(t *testing.T) {
	b := newBindings()
	b.set("refresh", chord{Mods: ui.Ctrl, Key: ui.KeyS})
	if got := b.get("stage"); len(got) != 0 {
		t.Fatalf("stage kept %v", got)
	}
	b.set("minimize", chord{Mods: ui.Ctrl, Key: ui.KeyU})
	if got := b.get("unstage"); len(got) != 1 || got[0].Key != ui.KeyU {
		t.Fatalf("window action touched repo action: %v", got)
	}
}

// Only changed actions are saved, and loading them restores the chords.
func TestBindingsSaveLoad(t *testing.T) {
	b := newBindings()
	b.set("stage", chord{Mods: ui.Ctrl, Key: ui.KeyG})
	saved := b.saved()
	if len(saved) != 1 || saved["stage"] != "Ctrl+G" {
		t.Fatalf("saved %v", saved)
	}
	loaded := newBindings()
	loaded.load(saved)
	if got := loaded.get("stage"); !chordsEqual(got, []chord{{ui.Ctrl, ui.KeyG}}) {
		t.Fatalf("loaded stage %v", got)
	}
	if got := loaded.get("unstage"); !chordsEqual(got, []chord{{ui.Ctrl, ui.KeyU}}) {
		t.Fatalf("default lost: %v", got)
	}
	bad := newBindings()
	bad.load(map[string]string{"stage": "Ctrl+Nope"})
	if bad.warn == "" || !chordsEqual(bad.get("stage"), []chord{{ui.Ctrl, ui.KeyS}}) {
		t.Fatalf("bad value: warn=%q stage=%v", bad.warn, bad.get("stage"))
	}
}

// Recording saves the chord and persists only the difference.
func TestCaptureKeyPersists(t *testing.T) {
	store, err := settings.Open("")
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(store)
	app.keys.capture = "stage"
	if !app.captureKey(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyG, Mods: ui.Ctrl}) {
		t.Fatal("captureKey did not handle the key")
	}
	if app.keys.capture != "" {
		t.Fatal("still recording")
	}
	if got := app.keys.get("stage"); !chordsEqual(got, []chord{{ui.Ctrl, ui.KeyG}}) {
		t.Fatalf("stage %v", got)
	}
	if got := store.Config().Shortcuts["stage"]; got != "Ctrl+G" {
		t.Fatalf("settings saved %q", got)
	}
}

// A rebound window action dispatches, and Ctrl+Q still minimizes by
// default.
func TestCustomShortcutDispatches(t *testing.T) {
	dir := newRepo(t)
	app := NewApp(nil)
	if err := app.open(dir); err != nil {
		t.Fatal(err)
	}
	minimized := 0
	app.minimize = func() { minimized++ }
	app.keys.set("minimize", chord{Mods: ui.Ctrl, Key: ui.KeyG})
	tt := ui.NewTester(app.View, 1100, 800)
	tt.Frame()
	tt.Key(ui.Ctrl, ui.KeyG)
	if minimized != 1 {
		t.Fatalf("Ctrl+G minimized %d times", minimized)
	}
	tt.Key(ui.Ctrl, ui.KeyQ)
	if minimized != 1 {
		t.Fatalf("old Ctrl+Q binding still minimized: %d", minimized)
	}
}

// The settings dialog lists the actions and their current chords.
func TestSettingsShowsShortcuts(t *testing.T) {
	app := NewApp(nil)
	app.settingsOpen = true
	app.settingsTab = 2
	tt := ui.NewTester(app.View, 1280, 820)
	tt.Frame()
	for _, want := range []string{"快捷键", "最小化窗口", "暂存选中", "录制", "恢复默认", "Ctrl+Q"} {
		if !tt.HasText(want) {
			t.Fatalf("settings missing %q: %v", want, tt.Texts())
		}
	}
}

// The footer shows a rebound action instead of the default.
func TestFooterUsesKeymap(t *testing.T) {
	dir := newRepo(t)
	r := NewRoot(dir)
	defer r.stopWatch()
	r.keys = newBindings()
	r.keys.set("stage", chord{Mods: ui.Ctrl, Key: ui.KeyG})
	tt := ui.NewTester(r.View, 1100, 800)
	tt.Frame()
	if !tt.HasText("Ctrl+G 暂存") {
		t.Fatalf("footer missing rebound hint: %v", tt.Texts())
	}
}
