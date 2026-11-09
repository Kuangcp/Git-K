package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"

	"github.com/egoist/mygo"
	mygoUI "github.com/egoist/mygo/ui"

	"git-k/ui"
)

//go:embed assets/icon-256.png
var appIconPNG []byte

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "打印版本号后退出")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "用法: git-k [仓库路径]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("git-k", version)
		return
	}

	dir := flag.Arg(0)
	if len(flag.Args()) > 1 {
		flag.Usage()
		return
	}

	mygo.App.WhenReady(func() {
		r := ui.NewRoot(dir)
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
