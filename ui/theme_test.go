package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"git-k/app/settings"
)

func TestResolveTheme(t *testing.T) {
	base := ui.LightTheme()
	got := resolveTheme(base, settings.Theme{Mode: settings.ThemeDark, Scheme: settings.SchemeOcean}, ui.Preferences{})
	if !got.Dark {
		t.Fatal("dark mode not applied")
	}
	if want := ui.Hex("#0f1822"); got.Background != want {
		t.Fatalf("ocean dark background: %v, want %v", got.Background, want)
	}
	light := resolveTheme(&ui.Theme{Dark: true}, settings.Theme{Mode: settings.ThemeLight, Scheme: settings.SchemeDefault}, ui.Preferences{})
	if light.Dark {
		t.Fatal("light mode not applied")
	}
}

func TestWindowBackgroundColor(t *testing.T) {
	if got, want := WindowBackgroundColor(settings.Theme{Mode: settings.ThemeDark, Scheme: settings.SchemeForest}), "#0e1712"; got != want {
		t.Fatalf("dark background: %q, want %q", got, want)
	}
	if got := WindowBackgroundColor(settings.DefaultTheme()); got != "light-dark(#f8f8f8, #1a1a1a)" {
		t.Fatalf("system background: %q", got)
	}
}

func TestSettingsThemeModal(t *testing.T) {
	store, err := settings.Open("")
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(store)
	tt := ui.NewTester(app.View, 1100, 800)
	tt.Frame()
	if err := tt.Click("设置"); err != nil {
		t.Fatalf("click settings: %v", err)
	}
	tt.Frame()
	if !tt.HasText("主题模式") || !tt.HasText("配色方案") {
		t.Fatalf("settings modal missing: %v", tt.Texts())
	}
	if err := tt.Click("深色"); err != nil {
		t.Fatalf("click dark: %v", err)
	}
	tt.Frame()
	if got := store.Config().Theme.Mode; got != settings.ThemeDark {
		t.Fatalf("saved mode: %q", got)
	}
}
