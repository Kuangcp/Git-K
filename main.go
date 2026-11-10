package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/egoist/mygo"
	mygoUI "github.com/egoist/mygo/ui"

	"git-k/app/settings"
	"git-k/ui"
)

//go:embed assets/icon-256.png
var appIconPNG []byte

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "打印版本号后退出")
	openDir := flag.String("d", "", "打开指定目录所在的 Git 仓库")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "用法: git-k [-d 仓库目录] [仓库路径]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("git-k", version)
		return
	}

	if len(flag.Args()) > 1 {
		flag.Usage()
		os.Exit(1)
	}
	dir, ok := repoDir(*openDir, flag.Arg(0))
	if !ok {
		flag.Usage()
		os.Exit(1)
	}

	store, err := settings.Open(settings.DefaultPath())
	if err != nil {
		log.Printf("读取设置失败，使用默认值: %v", err)
		store, _ = settings.Open("")
	}
	saveSettings := func() {
		if err := store.Flush(); err != nil {
			log.Printf("保存设置失败: %v", err)
		}
	}
	defer saveSettings()
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { saveSettings() })

	mygo.App.WhenReady(func() {
		r := ui.NewRootWithSettings(dir, store)
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "git-k",
			Width:     1280,
			Height:    820,
			MinWidth:  760,
			MinHeight: 520,
			Content:   mygoUI.View(r.View),
		})
		if err := win.SetIcon(appIconPNG); err != nil {
			log.Printf("设置窗口图标失败: %v", err)
		}
		if mygo.App.Dock != nil {
			_ = mygo.App.Dock.SetIcon(appIconPNG)
		}
		r.Attach(win)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// repoDir picks the directory to open: -d wins over a positional path,
// and giving both is an error.
func repoDir(openDir, arg string) (string, bool) {
	if openDir != "" && arg != "" {
		return "", false
	}
	if openDir != "" {
		return openDir, true
	}
	return arg, true
}
