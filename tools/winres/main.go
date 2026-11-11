// Command winres writes the Windows icon file and the .syso resources that
// embed it in git-k.exe. A pinned taskbar shortcut uses the executable's
// icon resource; setting the window icon at runtime is not enough.
//
// The COFF resource layout follows mygo's winres (MIT License,
// https://github.com/egoist/mygo).
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"

	"golang.org/x/image/draw"
)

var sizes = []int{16, 24, 32, 48, 64, 128, 256}

func main() {
	src, err := os.ReadFile("assets/icon-256.png")
	if err != nil {
		fail(err)
	}
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		fail(err)
	}
	images := make([][]byte, len(sizes))
	for i, size := range sizes {
		var b bytes.Buffer
		if err := png.Encode(&b, resize(img, size)); err != nil {
			fail(err)
		}
		images[i] = b.Bytes()
	}
	if err := os.WriteFile("assets/icon.ico", ico(images), 0o644); err != nil {
		fail(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		syso, err := resources(images, arch)
		if err != nil {
			fail(err)
		}
		name := fmt.Sprintf("icon_windows_%s.syso", arch)
		if err := os.WriteFile(name, syso, 0o644); err != nil {
			fail(err)
		}
	}
}

func resize(img image.Image, size int) image.Image {
	b := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}

// ico packs the PNGs as a Windows icon; Vista and later read PNG entries.
func ico(images [][]byte) []byte {
	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, [3]uint16{0, 1, uint16(len(images))})
	offset := 6 + 16*len(images)
	for i, p := range images {
		size := byte(sizes[i])
		if sizes[i] >= 256 {
			size = 0
		}
		out.Write([]byte{size, size, 0, 0})
		_ = binary.Write(&out, binary.LittleEndian, [2]uint16{1, 32})
		_ = binary.Write(&out, binary.LittleEndian, [2]uint32{uint32(len(p)), uint32(offset)})
		offset += len(p)
	}
	for _, p := range images {
		out.Write(p)
	}
	return out.Bytes()
}

// resources builds the .rsrc section of a COFF object with the icon group.
func resources(images [][]byte, goarch string) ([]byte, error) {
	arch, ok := map[string]struct{ machine, relocation uint16 }{
		"amd64": {0x8664, 0x0003}, // IMAGE_REL_AMD64_ADDR32NB
		"arm64": {0xAA64, 0x0002}, // IMAGE_REL_ARM64_ADDR32NB
	}[goarch]
	if !ok {
		return nil, fmt.Errorf("no Windows resources for %s", goarch)
	}
	const (
		rtIcon      = 3
		rtGroupIcon = 14
		lang        = 0x0409
	)

	var group bytes.Buffer
	_ = binary.Write(&group, binary.LittleEndian, [3]uint16{0, 1, uint16(len(images))})
	res := map[uint16]map[uint16][]byte{rtIcon: {}, rtGroupIcon: {1: nil}}
	for i, p := range images {
		id := uint16(i + 1)
		res[rtIcon][id] = p
		size := byte(sizes[i])
		if sizes[i] >= 256 {
			size = 0
		}
		group.Write([]byte{size, size, 0, 0})
		_ = binary.Write(&group, binary.LittleEndian, [2]uint16{1, 32})
		_ = binary.Write(&group, binary.LittleEndian, uint32(len(p)))
		_ = binary.Write(&group, binary.LittleEndian, id)
	}
	res[rtGroupIcon][1] = group.Bytes()

	types := sortedKeys(res)
	dirSize := func(n int) int { return 16 + 8*n }
	size := dirSize(len(types))
	for _, t := range types {
		size += dirSize(len(res[t])) + len(res[t])*dirSize(1)
	}
	entriesAt := size
	count := 0
	for _, t := range types {
		count += len(res[t])
	}
	dataAt := entriesAt + 16*count

	raw := make([]byte, dataAt)
	le := binary.LittleEndian
	dir := func(at, n int) { le.PutUint16(raw[at+14:], uint16(n)) }
	var relocs []uint32
	var data bytes.Buffer
	dir(0, len(types))
	next := dirSize(len(types))
	entry := entriesAt
	for i, t := range types {
		le.PutUint32(raw[16+8*i:], uint32(t))
		le.PutUint32(raw[16+8*i+4:], uint32(next)|0x80000000)
		ids := sortedKeys(res[t])
		idDir := next
		dir(idDir, len(ids))
		next += dirSize(len(ids))
		for j, id := range ids {
			le.PutUint32(raw[idDir+16+8*j:], uint32(id))
			le.PutUint32(raw[idDir+16+8*j+4:], uint32(next)|0x80000000)
			dir(next, 1)
			le.PutUint32(raw[next+16:], lang)
			le.PutUint32(raw[next+20:], uint32(entry))
			next += dirSize(1)

			for data.Len()%8 != 0 {
				data.WriteByte(0)
			}
			le.PutUint32(raw[entry:], uint32(dataAt+data.Len()))
			le.PutUint32(raw[entry+4:], uint32(len(res[t][id])))
			relocs = append(relocs, uint32(entry))
			entry += 16
			data.Write(res[t][id])
		}
	}
	raw = append(raw, data.Bytes()...)
	for len(raw)%4 != 0 {
		raw = append(raw, 0)
	}

	var out bytes.Buffer
	const headerSize, sectionHeaderSize = 20, 40
	rawAt := headerSize + sectionHeaderSize
	relocAt := rawAt + len(raw)
	symbolsAt := relocAt + 10*len(relocs)
	_ = binary.Write(&out, le, struct {
		Machine, Sections                   uint16
		Time, Symbols, SymbolCount          uint32
		OptionalHeaderSize, Characteristics uint16
	}{arch.machine, 1, 0, uint32(symbolsAt), 1, 0, 0})
	_ = binary.Write(&out, le, struct {
		Name                                        [8]byte
		VirtualSize, VirtualAddress, RawSize, RawAt uint32
		RelocAt, LineNumbersAt                      uint32
		Relocs, LineNumbers                         uint16
		Characteristics                             uint32
	}{[8]byte{'.', 'r', 's', 'r', 'c'}, 0, 0, uint32(len(raw)), uint32(rawAt), uint32(relocAt), 0, uint16(len(relocs)), 0, 0x40000040})
	out.Write(raw)
	for _, r := range relocs {
		_ = binary.Write(&out, le, r)
		_ = binary.Write(&out, le, uint32(0))
		_ = binary.Write(&out, le, arch.relocation)
	}
	out.Write([]byte{'.', 'r', 's', 'r', 'c', 0, 0, 0})
	_ = binary.Write(&out, le, uint32(0))
	_ = binary.Write(&out, le, uint16(1))
	_ = binary.Write(&out, le, uint16(0))
	out.Write([]byte{3, 0})
	_ = binary.Write(&out, le, uint32(4))
	return out.Bytes(), nil
}

func sortedKeys[V any](m map[uint16]V) []uint16 {
	keys := make([]uint16, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "winres: %v\n", err)
	os.Exit(1)
}

func init() {
	// Run from the repository root, as make does; keep paths predictable
	// if someone runs it from tools/winres.
	if _, err := os.Stat("assets/icon-256.png"); err != nil {
		if wd, err := os.Getwd(); err == nil {
			root := filepath.Join(wd, "..", "..")
			if _, err := os.Stat(filepath.Join(root, "assets", "icon-256.png")); err == nil {
				_ = os.Chdir(root)
			}
		}
	}
}
