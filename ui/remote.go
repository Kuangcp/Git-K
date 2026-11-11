package ui

import (
	"context"
	"strings"
	"time"
)

// pull brings the upstream into the current branch.
func (a *root) pull() {
	if a.busy || a.commitBusy || a.repo == nil {
		return
	}
	a.setBusy(true)
	repo := a.repo
	a.background(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		out, err := repo.Pull(ctx)
		cancel()
		a.update(func() {
			a.setBusy(false)
			a.diffKey = ""
			if err != nil {
				a.setNote("拉取失败：" + err.Error())
				a.refreshStatus()
				return
			}
			note := "拉取成功"
			if out != "" {
				note = "拉取成功：" + strings.Split(out, "\n")[0]
			}
			a.setNote(note)
			a.refreshStatus()
		})
	})
}

// push sends the current branch to its upstream.
func (a *root) push() {
	if a.busy || a.commitBusy || a.repo == nil {
		return
	}
	a.setBusy(true)
	repo := a.repo
	a.background(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		out, err := repo.Push(ctx)
		cancel()
		a.update(func() {
			a.setBusy(false)
			if err != nil {
				a.setNote("推送失败：" + err.Error())
				return
			}
			note := "推送成功"
			if out != "" {
				note = "推送成功：" + strings.Split(out, "\n")[0]
			}
			a.setNote(note)
			a.refreshStatus()
		})
	})
}
