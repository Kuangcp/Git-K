package ui

import "time"

var noteLifetime = 3 * time.Second

func (a *root) setBusy(busy bool) {
	a.busy = busy
	if a.win != nil {
		a.win.Invalidate()
	}
}

// setNote shows a short status message and schedules its removal.
func (a *root) setNote(note string) {
	a.note = note
	a.noteUntil = time.Now().Add(noteLifetime)
	if note == "" || a.win == nil {
		return
	}
	win := a.win
	time.AfterFunc(noteLifetime, func() {
		win.Update(func() { a.expireNote() })
	})
}

func (a *root) expireNote() {
	if a.note != "" && !time.Now().Before(a.noteUntil) {
		a.note = ""
	}
}
