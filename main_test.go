package main

import (
	"bytes"
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo"

	"git-k/app/settings"
)

// TestAppIconPixels 验证图标真的画出来了：浅色圆角底 + 紫→蓝渐变的 K。
func TestParseArgs(t *testing.T) {
	for _, c := range []struct {
		name    string
		args    []string
		wd      string
		wantDir string
		wantVer bool
		wantErr bool
	}{
		{"cwd", nil, "/work", "/work", false, false},
		{"d relative", []string{"-d", "test-repo"}, "/work", "/work/test-repo", false, false},
		{"d absolute", []string{"-d", "/tmp/repo"}, "/work", "/tmp/repo", false, false},
		{"positional", []string{"repo"}, "/work", "/work/repo", false, false},
		{"version", []string{"-version"}, "/work", "/work", true, false},
		{"two paths", []string{"a", "b"}, "/work", "", false, true},
		{"d and path", []string{"-d", "a", "b"}, "/work", "", false, true},
		{"unknown flag", []string{"-nope"}, "/work", "", false, true},
	} {
		got, err := parseArgs(c.args, c.wd)
		if (err != nil) != c.wantErr {
			t.Fatalf("%s: err=%v, wantErr=%v", c.name, err, c.wantErr)
		}
		if err != nil {
			continue
		}
		if got.dir != c.wantDir || got.version != c.wantVer {
			t.Fatalf("%s: %+v, want dir=%q version=%v", c.name, got, c.wantDir, c.wantVer)
		}
	}
}

func TestWindowPosition(t *testing.T) {
	displays := []mygo.Display{{WorkArea: mygo.Rectangle{X: 0, Y: 0, Width: 1920, Height: 1080}}}
	for _, c := range []struct {
		name string
		w    settings.Window
		x, y int
		ok   bool
	}{
		{"unpositioned", settings.Window{Width: 800, Height: 600}, 0, 0, false},
		{"onscreen", settings.Window{X: 100, Y: 80, Width: 800, Height: 600, Positioned: true}, 100, 80, true},
		{"partly visible", settings.Window{X: -50, Y: -20, Width: 800, Height: 600, Positioned: true}, -50, -20, true},
		{"offscreen", settings.Window{X: 3000, Y: 2000, Width: 800, Height: 600, Positioned: true}, 0, 0, false},
	} {
		x, y, ok := windowPosition(c.w, displays)
		if x != c.x || y != c.y || ok != c.ok {
			t.Fatalf("%s: got %d,%d,%v; want %d,%d,%v", c.name, x, y, ok, c.x, c.y, c.ok)
		}
	}
}

func TestAppIconPixels(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(appIconPNG))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() != 256 || b.Dy() != 256 {
		t.Fatalf("size: %v", b)
	}
	at := func(x, y int) (uint8, uint8, uint8, uint8) {
		r, g, bl, a := img.At(x, y).RGBA()
		return uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), uint8(a >> 8)
	}
	if _, _, _, a := at(0, 0); a > 60 {
		t.Fatalf("corner must be transparent, got a=%d", a)
	}
	if r, g, bl, a := at(128, 20); a < 200 || r < 245 || g < 245 || bl < 250 {
		t.Fatalf("background must be light, got %d,%d,%d a=%d", r, g, bl, a)
	}
	// 三条描边的渐变：上端偏紫，下端偏蓝青，中点居中。
	tr, tg, tb, _ := at(89, 73)
	br, bg, bb, _ := at(167, 183)
	mr, mg, mb, _ := at(89, 128)
	if tr < 100 || tr > 140 || tg > 110 || tb < 245 {
		t.Fatalf("K top must be purple, got %d,%d,%d", tr, tg, tb)
	}
	if br > 100 || bg < 150 || bb < 240 {
		t.Fatalf("K bottom must be blue-cyan, got %d,%d,%d", br, bg, bb)
	}
	if mr < 90 || mr > 135 || mg < 85 || mg > 125 || mb < 245 {
		t.Fatalf("K joint must be blue-violet, got %d,%d,%d", mr, mg, mb)
	}
	if tr == br && tg == bg && tb == bb {
		t.Fatal("K must be a gradient, not a flat color")
	}
	// K 右侧开口内是浅色底。
	if r, g, bl, _ := at(140, 128); r < 245 || g < 245 || bl < 250 {
		t.Fatalf("K counter must be light, got %d,%d,%d", r, g, bl)
	}
	for _, path := range []string{"assets/icon.svg", "assets/icon-256.png", "assets/icon-512.png"} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("icon source missing: %v", err)
		}
	}
}
