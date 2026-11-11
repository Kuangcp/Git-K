package ui

import (
	"context"
	"strings"
	"time"
)

func (a *root) doCommit() {
	if a.commitBusy || a.repo == nil {
		return
	}
	msg := a.message()
	if strings.TrimSpace(a.summary) == "" {
		a.commitErr = "请填写提交摘要"
		return
	}
	if !a.status.HasStaged() && !a.amendMode {
		a.commitErr = "没有已暂存的改动"
		return
	}
	a.commitBusy = true
	a.commitErr = ""
	repo := a.repo
	amend := a.amendMode
	a.background(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		out, err := repo.Commit(ctx, msg, amend)
		cancel()
		a.update(func() {
			a.commitBusy = false
			if err != nil {
				a.commitErr = err.Error()
				return
			}
			a.commitErr = ""
			note := "提交成功"
			a.setNote(note)
			for _, line := range strings.Split(out, "\n") {
				if strings.HasPrefix(line, "[") {
					a.setNote("已提交 " + line)
					break
				}
			}
			a.draftSeq.Add(1)
			_ = repo.SaveCommitMessage("")
			a.summary = ""
			a.body.Reset("")
			a.draftTxt = ""
			a.amendMode = false
			a.diffKey = ""
			a.refreshStatus()
		})
	})
}

// loadAmend loads the last commit's message and resets the index to its
// tree, so editing and committing amends that commit.
func (a *root) loadAmend() {
	if a.busy || a.commitBusy || a.repo == nil {
		return
	}
	a.setBusy(true)
	repo := a.repo
	a.background(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		msg, err := repo.LoadAmend(ctx)
		cancel()
		a.update(func() {
			a.setBusy(false)
			if err != nil {
				a.setNote("加载上次提交失败：" + err.Error())
				return
			}
			summary, body := splitMessage(msg)
			a.summary = summary
			a.body.Reset(body)
			a.amendMode = true
			a.commitErr = ""
			a.setNote("已加载上次提交，提交时会修正它")
			a.draftTxt = ""
			a.diffKey = ""
			a.refreshStatus()
		})
	})
}

func (a *root) maybeSaveDraft() {
	if a.repo == nil {
		return
	}
	s := a.message()
	if s == a.draftTxt {
		return
	}
	a.commitErr = ""
	a.setNote("")
	a.draftTxt = s
	if a.win == nil {
		return
	}
	repo := a.repo
	seq := a.draftSeq.Add(1)
	go func() {
		time.Sleep(500 * time.Millisecond)
		if a.draftSeq.Load() != seq {
			return
		}
		_ = repo.SaveCommitMessage(s)
	}()
}

// message composes the summary and the extended description as git
// stores them.
func (a *root) message() string {
	summary := strings.TrimSpace(a.summary)
	body := strings.TrimRight(a.body.String(), "\r\n")
	if body == "" {
		return summary
	}
	return summary + "\n\n" + body + "\n"
}

// splitMessage reads a draft back into its summary and its description.
func splitMessage(s string) (summary, body string) {
	s = strings.TrimRight(s, "\r\n")
	if i := strings.Index(s, "\n\n"); i >= 0 {
		return s[:i], strings.TrimLeft(s[i+2:], "\r\n")
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}
