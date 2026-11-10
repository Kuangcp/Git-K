package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"git-k/app/settings"
)

func TestAppTabs(t *testing.T) {
	dir1 := newRepo(t)
	dir2 := newRepo(t)
	app := NewApp(nil)

	if err := app.open(dir1); err != nil {
		t.Fatal(err)
	}
	if len(app.tabs) != 1 || app.active != 0 {
		t.Fatalf("first tab: %d tabs, active %d", len(app.tabs), app.active)
	}
	// A subdirectory opens the tab of its repository.
	sub := filepath.Join(dir1, "src")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := app.open(sub); err != nil {
		t.Fatal(err)
	}
	if len(app.tabs) != 1 {
		t.Fatalf("subdirectory opened a new tab: %d", len(app.tabs))
	}

	if err := app.open(dir2); err != nil {
		t.Fatal(err)
	}
	if len(app.tabs) != 2 || app.active != 1 {
		t.Fatalf("second tab: %d tabs, active %d", len(app.tabs), app.active)
	}
	app.activate(0)
	if app.active != 0 {
		t.Fatalf("active tab: %d", app.active)
	}

	// Closing the active tab selects the next one.
	app.closeAt(0)
	if len(app.tabs) != 1 || app.active != 0 {
		t.Fatalf("after close: %d tabs, active %d", len(app.tabs), app.active)
	}
	// Closing the last tab leaves the empty state.
	app.closeAt(0)
	if len(app.tabs) != 0 || app.active != -1 {
		t.Fatalf("after last close: %d tabs, active %d", len(app.tabs), app.active)
	}
}

// Clicking a tab focuses it.
func TestAppTabClick(t *testing.T) {
	dir1 := newRepo(t)
	dir2 := newRepo(t)
	app := NewApp(nil)
	if err := app.open(dir1); err != nil {
		t.Fatal(err)
	}
	if err := app.open(dir2); err != nil {
		t.Fatal(err)
	}
	name := app.tabs[0].root.repo.Name
	tt := ui.NewTester(app.View, 1100, 800)
	tt.Frame()
	if err := tt.Click(name); err != nil {
		t.Fatalf("click tab: %v", err)
	}
	if app.active != 0 {
		t.Fatalf("active tab: %d, want 0", app.active)
	}
}

// A layout moved in one tab is what the next tab takes.
// The close half of a tab closes it.
func TestAppCloseButton(t *testing.T) {
	dir1 := newRepo(t)
	dir2 := newRepo(t)
	app := NewApp(nil)
	if err := app.open(dir1); err != nil {
		t.Fatal(err)
	}
	if err := app.open(dir2); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(app.View, 1100, 800)
	tt.Frame()
	if err := tt.Click("×"); err != nil {
		t.Fatalf("click close: %v", err)
	}
	tt.Frame()
	if len(app.tabs) != 1 || app.active != 0 {
		t.Fatalf("after close: %d tabs, active %d", len(app.tabs), app.active)
	}
}

func TestAppSharedLayout(t *testing.T) {
	dir1 := newRepo(t)
	dir2 := newRepo(t)
	store, err := settings.Open("")
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(store)
	if err := app.open(dir1); err != nil {
		t.Fatal(err)
	}
	if err := app.open(dir2); err != nil {
		t.Fatal(err)
	}
	first := app.tabs[0].root
	first.sidePercent = 61
	first.commitPercent = 37
	first.saveLayout()
	app.activate(1)
	second := app.tabs[1].root
	if second.sidePercent != 61 || second.commitPercent != 37 {
		t.Fatalf("second tab layout: %v / %v", second.sidePercent, second.commitPercent)
	}
}

// Opening a directory that is not a repository adds no tab and reports
// an error.
func TestAppOpenInvalid(t *testing.T) {
	app := NewApp(nil)
	if err := app.open(t.TempDir()); err == nil {
		t.Fatal("opened a directory that is not a repository")
	}
	if len(app.tabs) != 0 {
		t.Fatalf("invalid open added %d tabs", len(app.tabs))
	}
	if app.openErr == "" {
		t.Fatal("no error recorded")
	}
}

func TestAppView(t *testing.T) {
	dir := newRepo(t)
	app := NewApp(nil)
	if err := app.open(dir); err != nil {
		t.Fatal(err)
	}
	app.tabs[0].root.refreshStatus() // no window in a test: read at once
	tt := ui.NewTester(app.View, 1100, 800)
	tt.Frame()
	if !tt.HasText("a.txt") || !tt.HasText("未跟踪") || !tt.HasText("打开仓库…") {
		t.Fatalf("tab content missing: %v", tt.Texts())
	}
}
