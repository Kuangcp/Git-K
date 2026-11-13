package ui

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// chord is one key combination: modifiers plus a main key, as mygo's
// shortcut model uses them.
type chord struct {
	Mods ui.Modifiers
	Key  ui.Key
}

// keyScope is where an action is handled: the window or one repository.
type keyScope int

const (
	scopeWindow keyScope = iota
	scopeRepo
)

// keyAction is one bindable action. Its ID is what settings.json stores,
// so changing the ID breaks saved bindings.
type keyAction struct {
	ID    string
	Label string
	Scope keyScope
	Def   []chord
}

// keyActions lists every bindable action, in the settings dialog's order.
// The window actions run in App.shortcuts; the repository actions run in
// root.shortcuts.
var keyActions = []keyAction{
	{ID: "minimize", Label: "最小化窗口", Scope: scopeWindow,
		Def: []chord{{ui.Ctrl, ui.KeyQ}}},
	{ID: "closeTab", Label: "关闭当前标签", Scope: scopeWindow,
		Def: []chord{{ui.Ctrl, ui.KeyW}}},
	{ID: "nextTab", Label: "下一个标签", Scope: scopeWindow,
		Def: []chord{{ui.Ctrl, ui.KeyTab}}},
	{ID: "prevTab", Label: "上一个标签", Scope: scopeWindow,
		Def: []chord{{ui.Ctrl | ui.Shift, ui.KeyTab}}},

	{ID: "workspace", Label: "切换工作区", Scope: scopeRepo,
		Def: []chord{{ui.Ctrl, ui.Key1}}},
	{ID: "branches", Label: "切换分支视图", Scope: scopeRepo,
		Def: []chord{{ui.Ctrl, ui.Key2}}},

	{ID: "stage", Label: "暂存选中", Scope: scopeRepo,
		Def: []chord{{ui.Ctrl, ui.KeyS}}},
	{ID: "stageAll", Label: "全部暂存", Scope: scopeRepo,
		Def: []chord{{ui.Ctrl | ui.Shift, ui.KeyS}}},
	{ID: "unstage", Label: "取消暂存", Scope: scopeRepo,
		Def: []chord{{ui.Ctrl, ui.KeyU}}},
	{ID: "refresh", Label: "刷新状态", Scope: scopeRepo,
		Def: []chord{{ui.Ctrl, ui.KeyR}, {0, ui.KeyF5}}},
	{ID: "amend", Label: "修正上次提交", Scope: scopeRepo,
		Def: []chord{{ui.Ctrl, ui.KeyM}}},
	{ID: "push", Label: "推送", Scope: scopeRepo,
		Def: []chord{{ui.Ctrl, ui.KeyP}}},
	{ID: "pull", Label: "拉取", Scope: scopeRepo,
		Def: []chord{{ui.Ctrl, ui.KeyL}}},
	{ID: "commit", Label: "提交", Scope: scopeRepo,
		Def: []chord{{ui.Ctrl, ui.KeyEnter}}},
}

func actionByID(id string) (keyAction, bool) {
	for _, act := range keyActions {
		if act.ID == id {
			return act, true
		}
	}
	return keyAction{}, false
}

// bindings holds the current chords for every action, the action being
// recorded, and a load warning. A nil bindings means the defaults, so a
// bare root in tests works without settings.
type bindings struct {
	chords  map[string][]chord
	capture string
	warn    string
}

func newBindings() *bindings {
	b := &bindings{chords: make(map[string][]chord, len(keyActions))}
	for _, act := range keyActions {
		b.chords[act.ID] = append([]chord(nil), act.Def...)
	}
	return b
}

// load applies saved "Ctrl+Shift+S, F5" strings over the defaults. An
// unknown action or malformed value is ignored in favor of the default,
// with a warning shown in the settings dialog.
func (b *bindings) load(saved map[string]string) {
	for _, act := range keyActions {
		raw, ok := saved[act.ID]
		if !ok {
			continue
		}
		cs, err := parseChords(raw)
		if err != nil {
			b.warn = "快捷键 " + act.Label + " 无效（" + err.Error() + "），用默认值"
			continue
		}
		b.chords[act.ID] = cs
	}
}

// saved returns only the actions the user changed, in the settings.json
// form.
func (b *bindings) saved() map[string]string {
	out := make(map[string]string)
	for _, act := range keyActions {
		cur := b.get(act.ID)
		if chordsEqual(cur, act.Def) {
			continue
		}
		out[act.ID] = formatChords(cur)
	}
	return out
}

// get returns an action's current chords; a nil bindings returns the
// defaults.
func (b *bindings) get(id string) []chord {
	if b != nil {
		if cs, ok := b.chords[id]; ok {
			return cs
		}
	}
	if act, ok := actionByID(id); ok {
		return act.Def
	}
	return nil
}

// text is the chord list as shown in the settings and hints.
func (b *bindings) text(id string) string {
	cs := b.get(id)
	if len(cs) == 0 {
		return "未绑定"
	}
	return formatChords(cs)
}

// hit consumes the first of the action's chords pressed this frame.
func (b *bindings) hit(c *ui.Context, id string) bool {
	for _, ch := range b.get(id) {
		if c.Shortcut(ch.Mods, ch.Key) {
			return true
		}
	}
	return false
}

// capturing says the settings dialog is recording a chord, so dispatch
// must stay out of the way.
func (b *bindings) capturing() bool { return b != nil && b.capture != "" }

// set binds one chord to an action and unbinds it from the other actions
// of the same scope.
func (b *bindings) set(id string, ch chord) {
	act, ok := actionByID(id)
	if !ok {
		return
	}
	if b.chords == nil {
		b.chords = make(map[string][]chord, len(keyActions))
	}
	for _, other := range keyActions {
		if other.ID == id || other.Scope != act.Scope {
			continue
		}
		var kept []chord
		for _, cur := range b.get(other.ID) {
			if cur != ch {
				kept = append(kept, cur)
			}
		}
		b.chords[other.ID] = kept
	}
	b.chords[id] = []chord{ch}
}

// reset restores one action's defaults, or all of them for an empty id.
func (b *bindings) reset(id string) {
	for _, act := range keyActions {
		if id != "" && act.ID != id {
			continue
		}
		b.chords[act.ID] = append([]chord(nil), act.Def...)
	}
}

// hint is the settings dialog's recording prompt.
func (b *bindings) hint() string {
	if b == nil || b.capture == "" {
		return ""
	}
	if act, ok := actionByID(b.capture); ok {
		return "按下 " + act.Label + " 的新组合（Esc 取消）"
	}
	return ""
}

func chordsEqual(a, b []chord) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// formatChords joins chords as "Ctrl+Enter, F5"; empty means unbound.
func formatChords(cs []chord) string {
	parts := make([]string, 0, len(cs))
	for _, c := range cs {
		parts = append(parts, formatChord(c))
	}
	return strings.Join(parts, ", ")
}

// formatChord writes one chord as "Ctrl+Shift+F".
func formatChord(c chord) string {
	var b strings.Builder
	for _, m := range []struct {
		mod  ui.Modifiers
		name string
	}{{ui.Ctrl, "Ctrl"}, {ui.Alt, "Alt"}, {ui.Shift, "Shift"}, {ui.Super, "Super"}} {
		if c.Mods&m.mod != 0 {
			b.WriteString(m.name)
			b.WriteByte('+')
		}
	}
	b.WriteString(keyName(c.Key))
	return b.String()
}

// parseChords reads "Ctrl+Enter, F5"; an empty string unbinds.
func parseChords(s string) ([]chord, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var out []chord
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		c, err := parseChord(part)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// parseChord reads "Ctrl+Shift+F".
func parseChord(s string) (chord, error) {
	parts := strings.Split(s, "+")
	var c chord
	for i, raw := range parts {
		p := strings.TrimSpace(raw)
		if p == "" {
			return chord{}, errBadChord(s)
		}
		if i < len(parts)-1 {
			switch strings.ToLower(p) {
			case "ctrl", "control":
				c.Mods |= ui.Ctrl
			case "shift":
				c.Mods |= ui.Shift
			case "alt", "option":
				c.Mods |= ui.Alt
			case "super", "cmd", "command", "win":
				c.Mods |= ui.Super
			default:
				return chord{}, errBadChord(s)
			}
			continue
		}
		key, ok := parseKeyName(p)
		if !ok {
			return chord{}, errBadChord(s)
		}
		c.Key = key
	}
	if c.Key == 0 {
		return chord{}, errBadChord(s)
	}
	return c, nil
}

// keyName writes a key as the readable name parseKeyName reads back.
func keyName(k ui.Key) string {
	switch {
	case k >= ui.KeyA && k <= ui.KeyZ:
		return string(rune('A' + k - ui.KeyA))
	case k >= ui.Key0 && k <= ui.Key9:
		return string(rune('0' + k - ui.Key0))
	case k >= ui.KeyF1 && k <= ui.KeyF12:
		return "F" + strconv.Itoa(int(k-ui.KeyF1)+1)
	}
	if name, ok := keyNames[k]; ok {
		return name
	}
	return "?"
}

// keyNames names the non-alphanumeric keys a user is likely to bind.
var keyNames = map[ui.Key]string{
	ui.KeyEnter: "Enter", ui.KeyEscape: "Escape", ui.KeyBackspace: "Backspace",
	ui.KeyTab: "Tab", ui.KeySpace: "Space", ui.KeyDelete: "Delete", ui.KeyInsert: "Insert",
	ui.KeyHome: "Home", ui.KeyEnd: "End", ui.KeyPageUp: "PageUp", ui.KeyPageDown: "PageDown",
	ui.KeyLeft: "Left", ui.KeyRight: "Right", ui.KeyUp: "Up", ui.KeyDown: "Down",
	ui.KeyMinus: "-", ui.KeyEqual: "=", ui.KeyComma: ",", ui.KeyPeriod: ".",
	ui.KeySlash: "/", ui.KeySemicolon: ";", ui.KeyQuote: "'",
	ui.KeyBracketLeft: "[", ui.KeyBracketRight: "]", ui.KeyBackslash: `\`,
	ui.KeyBackquote: "`",
}

// parseKeyName is keyName's inverse.
func parseKeyName(s string) (ui.Key, bool) {
	if len(s) == 1 {
		r := s[0]
		switch {
		case r >= 'a' && r <= 'z':
			return ui.KeyA + ui.Key(r-'a'), true
		case r >= 'A' && r <= 'Z':
			return ui.KeyA + ui.Key(r-'A'), true
		case r >= '0' && r <= '9':
			return ui.Key0 + ui.Key(r-'0'), true
		}
	}
	if len(s) >= 2 && (s[0] == 'f' || s[0] == 'F') {
		if n, err := strconv.Atoi(s[1:]); err == nil && n >= 1 && n <= 12 {
			return ui.KeyF1 + ui.Key(n-1), true
		}
	}
	for k, name := range keyNames {
		if strings.EqualFold(name, s) {
			return k, true
		}
	}
	return 0, false
}

// chordError is a value that could not be parsed.
type chordError string

func (e chordError) Error() string { return "无法识别的键位: " + string(e) }

func errBadChord(s string) error { return chordError(s) }

// hintFor formats an action as "Ctrl+S 暂存", or nothing when unbound, so
// footer and tooltips stay empty instead of saying "未绑定".
func (b *bindings) hintFor(id, label string) string {
	if len(b.get(id)) == 0 {
		return ""
	}
	return b.text(id) + " " + label
}

// chordText is an action's binding for a tooltip, or "" when unbound.
func (b *bindings) chordText(id string) string {
	if len(b.get(id)) == 0 {
		return ""
	}
	return b.text(id)
}
