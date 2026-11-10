package main

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	mygoUI "github.com/egoist/mygo/ui"

	"git-k/app/settings"
	"git-k/ui"
)

//go:embed assets/icon-256.png
var appIconPNG []byte

var version = "dev"

// options is one launch's command line, whether from this process or
// forwarded by a single-instance handoff.
type options struct {
	dir     string
	version bool
}

func main() {
	wd, _ := os.Getwd()
	opts, err := parseArgs(os.Args[1:], wd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "git-k: %v\n", err)
		printUsage()
		os.Exit(2)
	}
	if opts.version {
		fmt.Println("Git-K", version)
		return
	}

	mygo.App.SetName("Git-K")
	if !mygo.App.RequestSingleInstanceLock() {
		return // the running instance got the arguments and focuses its tab
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

	app := ui.NewApp(store)
	mygo.App.OnSecondInstance(func(args []string, wd string) {
		next, err := parseArgs(args, wd)
		if err != nil {
			log.Printf("忽略启动参数: %v", err)
			app.OpenAndFocus(wd)
			return
		}
		app.OpenAndFocus(next.dir)
	})

	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "Git-K",
			Width:     1280,
			Height:    820,
			MinWidth:  760,
			MinHeight: 520,
			Content:   mygoUI.View(app.View),
		})
		if err := win.SetIcon(appIconPNG); err != nil {
			log.Printf("设置窗口图标失败: %v", err)
		}
		if mygo.App.Dock != nil {
			_ = mygo.App.Dock.SetIcon(appIconPNG)
		}
		app.Attach(win)
		app.OpenAndFocus(opts.dir)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// parseArgs reads -d, -version and a positional repository path. Relative
// paths are relative to wd, which is this process's or the launching
// process's directory.
func parseArgs(args []string, wd string) (options, error) {
	fs := flag.NewFlagSet("git-k", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	showVersion := fs.Bool("version", false, "打印版本号后退出")
	openDir := fs.String("d", "", "打开指定目录所在的 Git 仓库")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() > 1 {
		return options{}, errors.New("只能指定一个仓库路径")
	}
	if *openDir != "" && fs.NArg() > 0 {
		return options{}, errors.New("-d 与仓库路径只能给一个")
	}
	dir := *openDir
	if dir == "" && fs.NArg() == 1 {
		dir = fs.Arg(0)
	}
	if dir == "" {
		dir = wd
	}
	if dir != "" && wd != "" && !filepath.IsAbs(dir) {
		dir = filepath.Join(wd, dir)
	}
	return options{dir: dir, version: *showVersion}, nil
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "用法: git-k [-d 仓库目录] [仓库路径]")
	fmt.Fprintln(os.Stderr, "  -d string")
	fmt.Fprintln(os.Stderr, "    \t打开指定目录所在的 Git 仓库")
	fmt.Fprintln(os.Stderr, "  -version")
	fmt.Fprintln(os.Stderr, "    \t打印版本号后退出")
}
