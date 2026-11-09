// Command genicon 生成 git-k 的应用图标：输出 assets/icon-{256,512}.png。
//
// 几何与配色与 assets/icon.svg 一致：浅色圆角底 + 2px 细边，字母 K 由三条
// 52px 圆头描边线段组成，渐变从 #7C4DFF 流到 #38BDF8。位图用 x/image/vector
// 直接光栅化，不依赖外部 SVG 渲染器，任何构建机都能重现。
package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/image/vector"
)

const (
	viewBox  = 512.0
	tileR    = 112.0
	borderW  = 2.0
	iconBg   = 0xFBFBFF
	iconEdge = 0xE9E7FA

	kStroke = 52.0
	kStemX  = 178.0
	kTopY   = 146.0
	kJointY = 256.0
	kBotY   = 366.0
	kArmX   = 334.0
)

// 渐变与 icon.svg 的坐标一致：从 K 的几何 bbox 左上到右下。
var (
	gradX0 = 152.0
	gradY0 = 120.0
	gradX1 = 360.0
	gradY1 = 392.0
)

var iconStops = []struct {
	at  float64
	rgb uint32
}{
	{0.00, 0x7C4DFF},
	{0.55, 0x6A7BFF},
	{1.00, 0x38BDF8},
}

func main() {
	out := "assets"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		log.Fatal(err)
	}
	for _, size := range []int{512, 256} {
		if err := writeIcon(filepath.Join(out, "icon-"+strconv.Itoa(size)+".png"), size); err != nil {
			log.Fatalf("icon %d: %v", size, err)
		}
	}
}

// writeIcon 画一张 size×size 的图标。
func writeIcon(path string, size int) error {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	k := float64(size) / viewBox

	// 圆角底：先整块描边色，再向内缩 2px 画底色，圆角外保持透明。
	edge := newRaster(size)
	roundedRect(edge, 0, 0, viewBox*k, viewBox*k, tileR*k)
	fill(img, edge, hexColor(iconEdge))
	base := newRaster(size)
	roundedRect(base, borderW*k, borderW*k, (viewBox-borderW)*k, (viewBox-borderW)*k, (tileR-borderW)*k)
	fill(img, base, hexColor(iconBg))

	// K 字形的描边轮廓：三条圆头胶囊并集。
	mask := image.NewAlpha(img.Bounds())
	r := kStroke / 2 * k
	segments := [][4]float64{
		{kStemX, kTopY, kStemX, kBotY},
		{kStemX, kJointY, kArmX, kTopY},
		{kStemX, kJointY, kArmX, kBotY},
	}
	for _, s := range segments {
		x0, y0, x1, y1 := s[0]*k, s[1]*k, s[2]*k, s[3]*k
		body := newRaster(size)
		capsule(body, x0, y0, x1, y1, r)
		zapOver(mask, body)
		for _, p := range [][2]float64{{x0, y0}, {x1, y1}} {
			cap := newRaster(size)
			circle(cap, p[0], p[1], r, false)
			zapOver(mask, cap)
		}
	}

	// 沿 bbox 对角线铺三段渐变。
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			a := mask.AlphaAt(x, y).A
			if a == 0 {
				continue
			}
			gx := (float64(x) + 0.5) / k
			gy := (float64(y) + 0.5) / k
			t := ((gx-gradX0)/(gradX1-gradX0) + (gy-gradY0)/(gradY1-gradY0)) / 2
			blendOver(img, x, y, iconColor(t), uint32(a)*0x101)
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// capsule 画一条圆角矩形身体（两端的圆头由 circle 并上）。
func capsule(z *vector.Rasterizer, x0, y0, x1, y1, r float64) {
	dx, dy := x1-x0, y1-y0
	l := math.Hypot(dx, dy)
	if l == 0 {
		return
	}
	nx, ny := -dy/l*r, dx/l*r
	z.MoveTo(float32(x0+nx), float32(y0+ny))
	z.LineTo(float32(x1+nx), float32(y1+ny))
	z.LineTo(float32(x1-nx), float32(y1-ny))
	z.LineTo(float32(x0-nx), float32(y0-ny))
	z.ClosePath()
}

// iconColor 返回渐变在 t 处的颜色（sRGB 线性插值，和 SVG 一致）。
func iconColor(t float64) color.RGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	for i := 1; i < len(iconStops); i++ {
		if t <= iconStops[i].at {
			a, b := iconStops[i-1], iconStops[i]
			u := (t - a.at) / (b.at - a.at)
			return lerpColor(a.rgb, b.rgb, u)
		}
	}
	return hexColor(iconStops[len(iconStops)-1].rgb)
}

// newRaster 建一个尺寸已知的光栅器（必须先 Reset，否则 Draw 会 panic）。
func newRaster(size int) *vector.Rasterizer {
	z := &vector.Rasterizer{}
	z.Reset(size, size)
	return z
}

func fill(dst draw.Image, z *vector.Rasterizer, c color.RGBA) {
	z.Draw(dst, dst.Bounds(), image.NewUniform(c), image.Point{})
}

// zapOver 把形状以 Over 画到遮罩上（用于把多个形状并成同一字形）。
func zapOver(mask *image.Alpha, z *vector.Rasterizer) {
	z.DrawOp = draw.Over
	z.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})
}

func lerpColor(a, b uint32, t float64) color.RGBA {
	ar, ag, ab := float64(a>>16&0xFF), float64(a>>8&0xFF), float64(a&0xFF)
	br, bg, bb := float64(b>>16&0xFF), float64(b>>8&0xFF), float64(b&0xFF)
	return color.RGBA{
		R: uint8(ar + (br-ar)*t),
		G: uint8(ag + (bg-ag)*t),
		B: uint8(ab + (bb-ab)*t),
		A: 255,
	}
}

// blendOver 把颜色 c 以 alpha（0..0xFFFF）叠到像素上。
func blendOver(img *image.RGBA, x, y int, c color.RGBA, alpha uint32) {
	i := img.PixOffset(x, y)
	a := alpha
	inv := 0xFFFF - a
	img.Pix[i+0] = uint8((uint32(img.Pix[i+0])*inv + uint32(c.R)*a) / 0xFFFF)
	img.Pix[i+1] = uint8((uint32(img.Pix[i+1])*inv + uint32(c.G)*a) / 0xFFFF)
	img.Pix[i+2] = uint8((uint32(img.Pix[i+2])*inv + uint32(c.B)*a) / 0xFFFF)
	img.Pix[i+3] = 255
}

// roundedRect 画圆角矩形（四角三次贝塞尔）。
func roundedRect(z *vector.Rasterizer, x0, y0, x1, y1, r float64) {
	k := 0.5522847498 * r
	z.MoveTo(float32(x0+r), float32(y0))
	z.LineTo(float32(x1-r), float32(y0))
	z.CubeTo(float32(x1-r+k), float32(y0), float32(x1), float32(y0+r-k), float32(x1), float32(y0+r))
	z.LineTo(float32(x1), float32(y1-r))
	z.CubeTo(float32(x1), float32(y1-r+k), float32(x1-r+k), float32(y1), float32(x1-r), float32(y1))
	z.LineTo(float32(x0+r), float32(y1))
	z.CubeTo(float32(x0+r-k), float32(y1), float32(x0), float32(y1-r+k), float32(x0), float32(y1-r))
	z.LineTo(float32(x0), float32(y0+r))
	z.CubeTo(float32(x0), float32(y0+r-k), float32(x0+r-k), float32(y0), float32(x0+r), float32(y0))
	z.ClosePath()
}

// circle 画圆。
func circle(z *vector.Rasterizer, cx, cy, r float64, ccw bool) {
	k := 0.5522847498 * r
	if !ccw {
		z.MoveTo(float32(cx+r), float32(cy))
		z.CubeTo(float32(cx+r), float32(cy+k), float32(cx+k), float32(cy+r), float32(cx), float32(cy+r))
		z.CubeTo(float32(cx-k), float32(cy+r), float32(cx-r), float32(cy+k), float32(cx-r), float32(cy))
		z.CubeTo(float32(cx-r), float32(cy-k), float32(cx-k), float32(cy-r), float32(cx), float32(cy-r))
		z.CubeTo(float32(cx+k), float32(cy-r), float32(cx+r), float32(cy-k), float32(cx+r), float32(cy))
		z.ClosePath()
		return
	}
	z.MoveTo(float32(cx+r), float32(cy))
	z.CubeTo(float32(cx+r), float32(cy-k), float32(cx+k), float32(cy-r), float32(cx), float32(cy-r))
	z.CubeTo(float32(cx-k), float32(cy-r), float32(cx-r), float32(cy-k), float32(cx-r), float32(cy))
	z.CubeTo(float32(cx-r), float32(cy+k), float32(cx-k), float32(cy+r), float32(cx), float32(cy+r))
	z.CubeTo(float32(cx+k), float32(cy+r), float32(cx+r), float32(cy+k), float32(cx+r), float32(cy))
	z.ClosePath()
}

// hexColor 把 0xRRGGBB 常量转成颜色（先转 uint32 变量，避免未类型化常量溢出 byte）。
func hexColor(v uint32) color.RGBA {
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}
