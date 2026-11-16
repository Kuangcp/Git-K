package ui

import (
	"github.com/egoist/mygo/ui"

	"git-k/app/settings"
)

// settingsSection is one entry in the settings dialog's left navigation.
type settingsSection struct {
	label string
	build func(*ui.Context)
}

// settingsSections lists the dialog's sections in navigation order.
func (a *App) settingsSections() []settingsSection {
	return []settingsSection{
		{"外观", a.themeSection},
		{"渲染", a.renderSection},
		{"快捷键", a.keymapSection},
	}
}

// settingsModal shows sections on the left and the chosen section's
// settings on the right, as db-g's settings window does.
func (a *App) settingsModal(c *ui.Context) {
	ui.Modal(c, &a.settingsOpen, func() {
		t := c.Theme()
		sections := a.settingsSections()
		if a.settingsTab < 0 || a.settingsTab >= len(sections) {
			a.settingsTab = 0
		}
		ui.Column(c).Width(680).Gap(12).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "设置").FontSize(16).Bold()
				ui.Text(c, "").Grow(1)
				if ui.Button(c, "关闭").Clicked() {
					a.settingsOpen = false
					a.keys.capture = ""
				}
			})
			ui.Row(c).Gap(0).FillHeight().Children(func() {
				ui.Column(c).Width(150).FillHeight().
					BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Children(func() {
					for i, sec := range sections {
						if settingsNavRow(c, sec.label, i == a.settingsTab).Clicked() {
							a.settingsTab = i
							a.keys.capture = ""
							c.Invalidate()
						}
					}
					ui.Text(c, "").Grow(1)
				})
				ui.Scroll(c).Width(530).Height(400).Children(func() {
					ui.Column(c).FillWidth().Padding(0, 16, 0, 16).Gap(12).Children(func() {
						sections[a.settingsTab].build(c)
					})
				})
			})
		})
	})
}

// settingsNavRow is one left-side category: a flat row that highlights
// when chosen and under the pointer.
func settingsNavRow(c *ui.Context, label string, selected bool) ui.Element {
	t := c.Theme()
	row := ui.ButtonBase(c).Label(label).Padding(10, 10).
		Justify(ui.Start).AlignItems(ui.Center)
	row.BorderWidth(0, 0, 1, 0).BorderColor(t.Border)
	switch {
	case selected:
		row.Background(t.Accent.Alpha(0.12))
	case row.Hovered():
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		txt := ui.Text(c, label).SingleLine()
		if selected {
			txt.Bold()
		} else {
			txt.TextColor(t.TextMuted)
		}
	})
	return row
}

// themeSection holds the Lobe preset choices.
func (a *App) themeSection(c *ui.Context) {
	prefs := a.themePrefs()
	ui.Text(c, "主题模式").FontSize(12).TextColor(c.Theme().TextMuted)
	mode := themeModeIndex(prefs.Mode)
	if ui.Segmented(c, &mode, "跟随系统", "浅色", "深色").Changed() {
		switch mode {
		case 1:
			prefs.Mode = settings.ThemeLight
		case 2:
			prefs.Mode = settings.ThemeDark
		default:
			prefs.Mode = settings.ThemeSystem
		}
		a.setTheme(prefs, c)
	}
	ui.Text(c, "配色方案").FontSize(12).TextColor(c.Theme().TextMuted)
	labels := make([]string, len(themeSchemeOptions))
	for i, opt := range themeSchemeOptions {
		labels[i] = opt.label
	}
	scheme := themeSchemeIndex(prefs.Scheme)
	if ui.Segmented(c, &scheme, labels...).Changed() {
		prefs.Scheme = themeSchemeOptions[scheme].key
		a.setTheme(prefs, c)
	}
	ui.Text(c, "修改即时生效并保存在设置中。").FontSize(11).TextColor(c.Theme().TextMuted)
}

// renderSection chooses the GPU or the CPU rasterizer.
func (a *App) renderSection(c *ui.Context) {
	gpu := !a.softwareRender()
	ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, "启用 GPU 渲染").FontSize(12).Grow(1)
		if ui.Switch(c, &gpu).Changed() {
			a.setSoftwareRender(!gpu)
			c.Invalidate()
		}
	})
	ui.Text(c, "关闭后使用 CPU 渲染，重启 Git-K 生效；可规避部分 Linux/X11 驱动的黑屏闪烁。").
		FontSize(11).TextColor(c.Theme().TextMuted)
}

// softwareRender reports whether the CPU renderer was chosen.
func (a *App) softwareRender() bool {
	return a.settings != nil && a.settings.Config().SoftwareRender
}

// setSoftwareRender stores the renderer choice; the window's renderer is
// made once at startup, so the next launch applies it.
func (a *App) setSoftwareRender(software bool) {
	if a.settings == nil {
		return
	}
	a.settings.Update(func(cfg *settings.Config) { cfg.SoftwareRender = software })
}

// keymapSection lists every bindable action, records a new chord on the
// focused box, and restores defaults.
func (a *App) keymapSection(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "点击「录制」后按下新组合；Esc 取消录制。").
		FontSize(11).TextColor(t.TextMuted)
	if hint := a.keys.hint(); hint != "" {
		ui.Text(c, hint).FontSize(12).TextColor(t.Accent).SingleLine()
		// Focus pins the focus on the box for the whole recording, so the
		// next chord reaches captureKey instead of a button or editor.
		ui.Box(c).FillWidth().Height(20).Focusable().Focus().
			HandleInput(a.captureKey)
	}
	if a.keys.warn != "" {
		ui.Text(c, a.keys.warn).FontSize(12).TextColor(t.Danger).SingleLine()
	}
	for _, act := range keyActions {
		recording := a.keys.capture == act.ID
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			label := ui.Text(c, act.Label).FontSize(12).SingleLine()
			if recording {
				label.TextColor(t.Accent)
			}
			ui.Text(c, "").Grow(1)
			keyText, col := a.keys.text(act.ID), t.TextMuted
			if recording {
				keyText, col = "按下新组合…", t.Accent
			}
			ui.Text(c, keyText).FontSize(12).TextColor(col).SingleLine()
			if ui.Button(c, "录制").Disabled(recording).Clicked() {
				a.keys.capture = act.ID
				c.Invalidate()
			}
			if ui.Button(c, "恢复默认").Disabled(chordsEqual(a.keys.get(act.ID), act.Def)).Clicked() {
				a.resetChord(act.ID)
				c.Invalidate()
			}
		})
	}
	ui.Row(c).Gap(8).Children(func() {
		if ui.Button(c, "全部恢复默认").Clicked() {
			a.resetChord("")
			c.Invalidate()
		}
		if a.keys.capture != "" {
			if ui.Button(c, "取消录制").Clicked() {
				a.keys.capture = ""
				c.Invalidate()
			}
		}
	})
}

// captureKey records the next chord for the action being recorded. The
// focused box delivers both modifier-only and ordinary keys here.
func (a *App) captureKey(ev ui.InputEvent) bool {
	if a.keys.capture == "" || ev.Kind != ui.InputKeyDown {
		return false
	}
	if ev.Key == 0 {
		return true // a modifier alone; wait for the main key
	}
	if ev.Key == ui.KeyEscape && ev.Mods == 0 {
		a.keys.capture = ""
		return true
	}
	a.keys.set(a.keys.capture, chord{Mods: ev.Mods, Key: ev.Key})
	a.keys.capture = ""
	a.saveKeys()
	return true
}

// resetChord restores one action's defaults, or all of them for "".
func (a *App) resetChord(id string) {
	a.keys.reset(id)
	a.keys.capture = ""
	a.saveKeys()
}

// saveKeys persists only the actions that differ from the defaults.
func (a *App) saveKeys() {
	if a.settings == nil {
		return
	}
	saved := a.keys.saved()
	if len(saved) == 0 {
		saved = nil
	}
	a.settings.Update(func(cfg *settings.Config) { cfg.Shortcuts = saved })
}
