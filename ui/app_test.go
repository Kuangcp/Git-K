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

// Opening from a nested directory uses the repository's root tab, and
// dedupes against opening that root directly.
func TestOpenFromSubdir(t *testing.T) {
	root := newRepo(t)
	sub := filepath.Join(root, "one", "two", "three")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	app := NewApp(nil)
	if err := app.open(sub); err != nil {
		t.Fatal(err)
	}
	if len(app.tabs) != 1 || app.tabs[0].key != canonicalKey(root) {
		t.Fatalf("subdir tab key: %q, want %q", app.tabs[0].key, canonicalKey(root))
	}
	if err := app.open(root); err != nil {
		t.Fatal(err)
	}
	if len(app.tabs) != 1 {
		t.Fatalf("root opened a second tab: %d", len(app.tabs))
	}
}

// The active repository's name is centered in the title bar.
func TestTitleBarCenter(t *testing.T) {
	dir := newRepo(t)
	app := NewApp(nil)
	if err := app.open(dir); err != nil {
		t.Fatal(err)
	}
	name := app.activeName()
	tt := ui.NewTester(app.View, 1100, 800)
	tt.Frame()
	rect, ok := tt.Find(name)
	if !ok {
		t.Fatalf("title name missing: %v", tt.Texts())
	}
	center := rect.X + rect.W/2
	if center < 545 || center > 555 {
		t.Fatalf("title center: %v, want about 550", center)
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
// Opening repositories records them newest first and moves a reopened
// repository back to the front.
func TestAppRecent(t *testing.T) {
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
	recent := app.recent()
	if len(recent) != 2 || recent[0] != dir2 || recent[1] != dir1 {
		t.Fatalf("recent: %v, want [%s %s]", recent, dir2, dir1)
	}
	if err := app.open(dir1); err != nil {
		t.Fatal(err)
	}
	recent = app.recent()
	if len(recent) != 2 || recent[0] != dir1 || recent[1] != dir2 {
		t.Fatalf("reopened recent: %v", recent)
	}
}

// The home page lists recent repositories with the empty state.
func TestHomeView(t *testing.T) {
	dir := newRepo(t)
	store, err := settings.Open("")
	if err != nil {
		t.Fatal(err)
	}
	store.Update(func(c *settings.Config) { c.Recent = []string{dir} })
	app := NewApp(store)
	tt := ui.NewTester(app.View, 1100, 800)
	tt.Frame()
	if !tt.HasText("最近打开") || !tt.HasText(filepath.Base(dir)) || !tt.HasText(dir) {
		t.Fatalf("home missing recent: %v", tt.Texts())
	}
	if !tt.HasText("打开其他仓库…") {
		t.Fatalf("home missing open button: %v", tt.Texts())
	}
}

// The home page takes the app icon without failing to build.
func TestHomeIcon(t *testing.T) {
	data, err := os.ReadFile("../assets/icon-256.png")
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(nil)
	app.SetIconPNG(data)
	tt := ui.NewTester(app.View, 1100, 800)
	tt.Frame()
	if !tt.HasText("最近打开") || !tt.HasText("打开其他仓库…") {
		t.Fatalf("home missing: %v", tt.Texts())
	}
}

// An initial launch outside a repository queues a quiet open, and the
// home page never shows the failed path as an error.
func TestOpenInitialQuiet(t *testing.T) {
	app := NewApp(nil)
	app.OpenInitial(t.TempDir())
	if len(app.pending) != 1 || !app.pending[0].quiet {
		t.Fatalf("pending: %+v", app.pending)
	}
	app.openErr = "/home/zk 不是 Git 仓库"
	tt := ui.NewTester(app.View, 1100, 800)
	tt.Frame()
	if tt.HasText("不是 Git 仓库") {
		t.Fatalf("home shows the open error: %v", tt.Texts())
	}
}

// A recent item is one clickable two-line block.
func TestHomeRecentClick(t *testing.T) {
	dir := newRepo(t)
	store, err := settings.Open("")
	if err != nil {
		t.Fatal(err)
	}
	store.Update(func(c *settings.Config) { c.Recent = []string{dir} })
	app := NewApp(store)
	tt := ui.NewTester(app.View, 1100, 800)
	tt.Frame()
	if err := tt.Click(dir); err != nil {
		t.Fatalf("click recent: %v", err)
	}
	if len(app.pending) != 1 || app.pending[0].dir != dir {
		t.Fatalf("recent click did not open: %+v", app.pending)
	}
}

// The load popover lists history repositories that are not open.
func TestLoadRepoPopover(t *testing.T) {
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
	app.closeAt(1) // keep dir1 open, dir2 closed
	tt := ui.NewTester(app.View, 1100, 800)
	tt.Frame()
	if err := tt.Click("加载仓库"); err != nil {
		t.Fatalf("click load: %v", err)
	}
	tt.Frame()
	if !tt.HasText(filepath.Base(dir2)) {
		t.Fatalf("load popover missing closed repo: %v", tt.Texts())
	}
	if err := tt.Click(filepath.Base(dir2)); err != nil {
		t.Fatalf("click closed repo: %v", err)
	}
	tt.Frame()
	if len(app.pending) != 1 || app.pending[0].dir != dir2 {
		t.Fatalf("load did not open: tabs=%d pending=%+v", len(app.tabs), app.pending)
	}
}

// The home page removes a history entry from its context menu.
func TestHomeDeleteRecent(t *testing.T) {
	dir := newRepo(t)
	store, err := settings.Open("")
	if err != nil {
		t.Fatal(err)
	}
	store.Update(func(c *settings.Config) { c.Recent = []string{dir} })
	app := NewApp(store)
	tt := ui.NewTester(app.View, 1100, 800)
	tt.Frame()
	rect, ok := tt.Find(dir)
	if !ok {
		t.Fatalf("recent path missing: %v", tt.Texts())
	}
	tt.RightClickAt(rect.X+rect.W/2, rect.Y+rect.H/2)
	if err := tt.ChooseMenuItem("删除记录"); err != nil {
		t.Fatalf("choose delete: %v", err)
	}
	tt.Frame()
	if got := store.Config().Recent; len(got) != 0 {
		t.Fatalf("recent not removed: %v", got)
	}
	if tt.HasText(dir) {
		t.Fatalf("home still shows deleted recent: %v", tt.Texts())
	}
}

// Ctrl+W closes the active tab.
func TestAppCloseShortcut(t *testing.T) {
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
	tt.Key(ui.Ctrl, ui.KeyW)
	tt.Frame()
	if len(app.tabs) != 1 || app.active != 0 {
		t.Fatalf("Ctrl+W: %d tabs, active %d", len(app.tabs), app.active)
	}
}

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
	if !tt.HasText("a.txt") || !tt.HasText("未跟踪") || !tt.HasText("打开仓库") {
		t.Fatalf("tab content missing: %v", tt.Texts())
	}
}
