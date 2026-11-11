package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"git-k/app/settings"
	lobe "git-k/ui/lobe"
)

type themeSchemeOption struct {
	key, label string
}

var themeSchemeOptions = []themeSchemeOption{
	{settings.SchemeDefault, "默认"},
	{settings.SchemeOcean, "海洋"},
	{settings.SchemeForest, "森林"},
	{settings.SchemeWarm, "暖阳"},
}

type themePalette struct {
	Background, Surface, Border, Text, TextMuted string
}

type themePalettePair struct {
	light, dark themePalette
}

var themePalettes = map[string]themePalettePair{
	settings.SchemeDefault: {
		light: themePalette{Background: "#f8f8f8", Surface: "#ffffff", Border: "#e3e3e3", Text: "#1f1f1f", TextMuted: "#666666"},
		dark:  themePalette{Background: "#1a1a1a", Surface: "#242424", Border: "#343434", Text: "#e6e6e6", TextMuted: "#b4b4b4"},
	},
	settings.SchemeOcean: {
		light: themePalette{Background: "#edf4fb", Surface: "#ffffff", Border: "#d3e2f2", Text: "#17212b", TextMuted: "#5b6b7c"},
		dark:  themePalette{Background: "#0f1822", Surface: "#172433", Border: "#2a3d52", Text: "#e6f0fa", TextMuted: "#9fb2c6"},
	},
	settings.SchemeForest: {
		light: themePalette{Background: "#eef5ef", Surface: "#ffffff", Border: "#d2e3d5", Text: "#15231a", TextMuted: "#5a6c5f"},
		dark:  themePalette{Background: "#0e1712", Surface: "#16241c", Border: "#2a3e32", Text: "#e2efe6", TextMuted: "#9db2a3"},
	},
	settings.SchemeWarm: {
		light: themePalette{Background: "#faf4ec", Surface: "#fffdf9", Border: "#e8d9c8", Text: "#241b12", TextMuted: "#7a6958"},
		dark:  themePalette{Background: "#1a1410", Surface: "#251d16", Border: "#3b3026", Text: "#f1e7dc", TextMuted: "#b6a695"},
	},
}

// applyTheme sets the Lobe preset chosen in settings for this frame.
func (a *App) applyTheme(c *ui.Context) {
	c.SetTheme(resolveTheme(c.Theme(), a.themePrefs(), c.Preferences()))
}

func (a *App) themePrefs() settings.Theme {
	if a.settings == nil {
		return settings.DefaultTheme()
	}
	return a.settings.Config().Theme
}

func resolveTheme(base *ui.Theme, p settings.Theme, desk ui.Preferences) *ui.Theme {
	p = p.Normalize()
	dark := base != nil && base.Dark
	switch p.Mode {
	case settings.ThemeLight:
		dark = false
	case settings.ThemeDark:
		dark = true
	}
	var tokens *lobe.Tokens
	if dark {
		tokens = lobe.Dark()
	} else {
		tokens = lobe.Light()
	}
	t := *tokens.Theme()
	applyPalette(&t, paletteFor(p.Scheme, dark))
	scale := desk.TextScale
	if scale <= 0 {
		scale = 1
	}
	t.FontSize *= scale
	return &t
}

func paletteFor(scheme string, dark bool) themePalette {
	pair, ok := themePalettes[scheme]
	if !ok {
		pair = themePalettes[settings.SchemeDefault]
	}
	if dark {
		return pair.dark
	}
	return pair.light
}

func applyPalette(t *ui.Theme, p themePalette) {
	set := func(dst *ui.Color, hex string) {
		if hex = strings.TrimSpace(hex); hex != "" {
			*dst = ui.Hex(hex)
		}
	}
	set(&t.Background, p.Background)
	set(&t.Surface, p.Surface)
	set(&t.Border, p.Border)
	set(&t.Text, p.Text)
	set(&t.TextMuted, p.TextMuted)
}

// WindowBackgroundColor is the color behind a frameless window, following
// the desktop in system mode.
func WindowBackgroundColor(p settings.Theme) string {
	p = p.Normalize()
	light := paletteFor(p.Scheme, false).Background
	dark := paletteFor(p.Scheme, true).Background
	switch p.Mode {
	case settings.ThemeLight:
		return light
	case settings.ThemeDark:
		return dark
	}
	return "light-dark(" + light + ", " + dark + ")"
}

// setTheme persists and applies a preset; the frame is rebuilt with it.
func (a *App) setTheme(p settings.Theme, c *ui.Context) {
	p = p.Normalize()
	if a.settings != nil {
		a.settings.Update(func(cfg *settings.Config) { cfg.Theme = p })
	}
	if a.win != nil {
		_ = a.win.SetBackgroundColor(WindowBackgroundColor(p))
	}
	c.Invalidate()
}

func themeModeIndex(mode string) int {
	switch mode {
	case settings.ThemeLight:
		return 1
	case settings.ThemeDark:
		return 2
	}
	return 0
}

func themeSchemeIndex(scheme string) int {
	for i, opt := range themeSchemeOptions {
		if opt.key == scheme {
			return i
		}
	}
	return 0
}
