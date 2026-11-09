package main

import (
	"bytes"
	"image/png"
	"os"
	"testing"
)

// TestAppIconPixels 验证图标真的画出来了：浅色圆角底 + 紫→蓝渐变的 K。
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
