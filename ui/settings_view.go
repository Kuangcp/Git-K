package ui

import (
	"github.com/egoist/mygo/ui"

	"git-k/app/settings"
)

// settingsModal chooses one of the Lobe presets.
func (a *App) settingsModal(c *ui.Context) {
	ui.Modal(c, &a.settingsOpen, func() {
		prefs := a.themePrefs()
		ui.Column(c).Width(440).Gap(12).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "设置").FontSize(16).Bold()
				ui.Text(c, "").Grow(1)
				if ui.Button(c, "关闭").Clicked() {
					a.settingsOpen = false
				}
			})
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
		})
	})
}
