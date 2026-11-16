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
	applySoftwareRender(store)
	saveSettings := func() {
		if err := store.Flush(); err != nil {
			log.Printf("保存设置失败: %v", err)
		}
	}
	defer saveSettings()
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { saveSettings() })

	app := ui.NewApp(store)
	app.SetIconPNG(appIconPNG)
	mygo.App.OnSecondInstance(func(args []string, wd string) {
		next, err := parseArgs(args, wd)
		if err != nil {
			log.Printf("忽略启动参数: %v", err)
			app.OpenFocus(wd)
			return
		}
		app.OpenFocus(next.dir)
	})

	mygo.App.WhenReady(func() {
		cfg := store.Config()
		width, height := cfg.Window.Width, cfg.Window.Height
		if width < 760 {
			width = 760
		}
		if height < 520 {
			height = 520
		}
		wo := mygo.WindowOptions{
			Title:           "Git-K",
			Width:           width,
			Height:          height,
			MinWidth:        760,
			MinHeight:       520,
			Frameless:       true,
			CornerRadius:    10,
			BackgroundColor: ui.WindowBackgroundColor(cfg.Theme),
			Content:         mygoUI.View(app.View),
		}
		if x, y, ok := windowPosition(cfg.Window, mygo.Screen.Displays()); ok {
			wo.X, wo.Y = x, y
		}
		win := mygo.NewWindow(wo)
		if err := win.SetIcon(appIconPNG); err != nil {
			log.Printf("设置窗口图标失败: %v", err)
		}
		if mygo.App.Dock != nil {
			_ = mygo.App.Dock.SetIcon(appIconPNG)
		}
		app.Attach(win)
		app.OpenInitial(opts.dir)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// applySoftwareRender turns mygo's GPU renderer off when the setting asks
// for it. mygo reads MYGO_GPU when it makes a window's renderer, so setting
// it before the window runs is enough; the change applies on the next
// launch only, since a window keeps the renderer it made.
func applySoftwareRender(st *settings.Store) {
	if st != nil && st.Config().SoftwareRender {
		_ = os.Setenv("MYGO_GPU", "0")
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

// windowPosition restores a saved position when enough of the window is
// on a display; otherwise the window is centered.
func windowPosition(w settings.Window, displays []mygo.Display) (int, int, bool) {
	if !w.Positioned {
		return 0, 0, false
	}
	width, height := w.Width, w.Height
	if width <= 0 {
		width = 1280
	}
	if height <= 0 {
		height = 820
	}
	for _, d := range displays {
		wa := d.WorkArea
		left := max(w.X, wa.X)
		right := min(w.X+width, wa.X+wa.Width)
		top := max(w.Y, wa.Y)
		bottom := min(w.Y+height, wa.Y+wa.Height)
		if right-left >= 80 && bottom-top >= 40 {
			return w.X, w.Y, true
		}
	}
	return 0, 0, false
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "用法: git-k [-d 仓库目录] [仓库路径]")
	fmt.Fprintln(os.Stderr, "  -d string")
	fmt.Fprintln(os.Stderr, "    \t打开指定目录所在的 Git 仓库")
	fmt.Fprintln(os.Stderr, "  -version")
	fmt.Fprintln(os.Stderr, "    \t打印版本号后退出")
}
