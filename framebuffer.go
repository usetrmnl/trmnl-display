package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"strconv"
	"strings"
	"syscall"
)

type ColorDepth int

const (
	ColorDepth16 ColorDepth = 16
	ColorDepth24 ColorDepth = 24
	ColorDepth32 ColorDepth = 32
)

type Framebuffer struct {
	file       *os.File
	data       []byte
	width      int
	height     int
	stride     int
	colorDepth ColorDepth
	bounds     image.Rectangle
}

func OpenFramebuffer(device string) (*Framebuffer, error) {
	if device == "" {
		device = "/dev/fb0"
	}

	file, err := os.OpenFile(device, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open framebuffer device: %v", err)
	}

	fb := &Framebuffer{file: file}

	if err := fb.detectFormat(); err != nil {
		file.Close()
		return nil, err
	}

	if err := fb.mapMemory(); err != nil {
		file.Close()
		return nil, err
	}

	return fb, nil
}

func OpenFramebufferWithDepth(device string, depth ColorDepth) (*Framebuffer, error) {
	if device == "" {
		device = "/dev/fb0"
	}

	file, err := os.OpenFile(device, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open framebuffer device: %v", err)
	}

	fb := &Framebuffer{file: file}

	if err := fb.detectFormat(); err != nil {
		file.Close()
		return nil, err
	}

	// Override detected depth with forced depth
	if depth == ColorDepth16 || depth == ColorDepth24 || depth == ColorDepth32 {
		oldDepth := fb.colorDepth
		fb.colorDepth = depth
		fb.stride = fb.width * (int(fb.colorDepth) / 8)
		fmt.Printf("Overriding detected color depth %d with forced depth %d\n", oldDepth, depth)
	}

	if err := fb.mapMemory(); err != nil {
		file.Close()
		return nil, err
	}

	return fb, nil
}

func (fb *Framebuffer) detectFormat() error {
	bppPath := "/sys/class/graphics/fb0/bits_per_pixel"
	bppData, err := os.ReadFile(bppPath)
	if err != nil {
		return fmt.Errorf("failed to read bits_per_pixel: %v", err)
	}
	bpp, err := strconv.Atoi(strings.TrimSpace(string(bppData)))
	if err != nil {
		return fmt.Errorf("failed to parse bits_per_pixel: %v", err)
	}

	switch bpp {
	case 16:
		fb.colorDepth = ColorDepth16
	case 24:
		fb.colorDepth = ColorDepth24
	case 32:
		fb.colorDepth = ColorDepth32
	default:
		return fmt.Errorf("unsupported color depth: %d bits", bpp)
	}

	sizePath := "/sys/class/graphics/fb0/virtual_size"
	sizeData, err := os.ReadFile(sizePath)
	if err != nil {
		return fmt.Errorf("failed to read virtual_size: %v", err)
	}
	parts := strings.Split(strings.TrimSpace(string(sizeData)), ",")
	if len(parts) != 2 {
		return fmt.Errorf("invalid virtual_size format")
	}

	fb.width, err = strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("failed to parse width: %v", err)
	}

	fb.height, err = strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("failed to parse height: %v", err)
	}

	stridePath := "/sys/class/graphics/fb0/stride"
	strideData, err := os.ReadFile(stridePath)
	if err != nil {
		fb.stride = fb.width * (int(fb.colorDepth) / 8)
	} else {
		fb.stride, _ = strconv.Atoi(strings.TrimSpace(string(strideData)))
	}

	fb.bounds = image.Rect(0, 0, fb.width, fb.height)

	fmt.Printf("Framebuffer detected: %dx%d, %d bpp, stride: %d\n",
		fb.width, fb.height, fb.colorDepth, fb.stride)

	return nil
}

func (fb *Framebuffer) mapMemory() error {
	size := fb.stride * fb.height

	data, err := syscall.Mmap(
		int(fb.file.Fd()),
		0,
		size,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED,
	)
	if err != nil {
		return fmt.Errorf("failed to mmap framebuffer: %v", err)
	}

	fb.data = data
	return nil
}

func (fb *Framebuffer) Close() error {
	if fb.data != nil {
		syscall.Munmap(fb.data)
		fb.data = nil
	}
	if fb.file != nil {
		return fb.file.Close()
	}
	return nil
}

func (fb *Framebuffer) Bounds() image.Rectangle {
	return fb.bounds
}

func (fb *Framebuffer) ColorModel() color.Model {
	switch fb.colorDepth {
	case ColorDepth16:
		return color.RGBA64Model
	case ColorDepth24, ColorDepth32:
		return color.RGBAModel
	default:
		return color.RGBAModel
	}
}

func (fb *Framebuffer) At(x, y int) color.Color {
	if x < 0 || y < 0 || x >= fb.width || y >= fb.height {
		return color.RGBA{}
	}

	offset := y*fb.stride + x*(int(fb.colorDepth)/8)

	switch fb.colorDepth {
	case ColorDepth16:
		if offset+1 >= len(fb.data) {
			return color.RGBA{}
		}
		pixel := uint16(fb.data[offset]) | uint16(fb.data[offset+1])<<8
		return rgb565ToRGBA(pixel)

	case ColorDepth24:
		if offset+2 >= len(fb.data) {
			return color.RGBA{}
		}
		return color.RGBA{
			B: fb.data[offset],
			G: fb.data[offset+1],
			R: fb.data[offset+2],
			A: 255,
		}

	case ColorDepth32:
		if offset+3 >= len(fb.data) {
			return color.RGBA{}
		}
		return color.RGBA{
			B: fb.data[offset],
			G: fb.data[offset+1],
			R: fb.data[offset+2],
			A: fb.data[offset+3],
		}
	}

	return color.RGBA{}
}

func (fb *Framebuffer) Set(x, y int, c color.Color) {
	if x < 0 || y < 0 || x >= fb.width || y >= fb.height {
		return
	}

	r, g, b, a := c.RGBA()
	r8 := uint8(r >> 8)
	g8 := uint8(g >> 8)
	b8 := uint8(b >> 8)
	a8 := uint8(a >> 8)

	offset := y*fb.stride + x*(int(fb.colorDepth)/8)

	switch fb.colorDepth {
	case ColorDepth16:
		if offset+1 >= len(fb.data) {
			return
		}
		pixel := rgbaToRGB565(r8, g8, b8)
		fb.data[offset] = uint8(pixel)
		fb.data[offset+1] = uint8(pixel >> 8)

	case ColorDepth24:
		if offset+2 >= len(fb.data) {
			return
		}
		fb.data[offset] = b8
		fb.data[offset+1] = g8
		fb.data[offset+2] = r8

	case ColorDepth32:
		if offset+3 >= len(fb.data) {
			return
		}
		fb.data[offset] = b8
		fb.data[offset+1] = g8
		fb.data[offset+2] = r8
		fb.data[offset+3] = a8
	}
}

func rgb565ToRGBA(pixel uint16) color.RGBA {
	r := uint8((pixel&0xF800)>>11) << 3
	g := uint8((pixel&0x07E0)>>5) << 2
	b := uint8(pixel&0x001F) << 3

	r |= r >> 5
	g |= g >> 6
	b |= b >> 5

	return color.RGBA{R: r, G: g, B: b, A: 255}
}

func rgbaToRGB565(r, g, b uint8) uint16 {
	r5 := uint16(r>>3) & 0x1F
	g6 := uint16(g>>2) & 0x3F
	b5 := uint16(b>>3) & 0x1F

	return (r5 << 11) | (g6 << 5) | b5
}

func (fb *Framebuffer) SubImage(r image.Rectangle) draw.Image {
	r = r.Intersect(fb.bounds)
	if r.Empty() {
		return &subFramebuffer{
			fb:     fb,
			bounds: image.Rectangle{},
		}
	}
	return &subFramebuffer{
		fb:     fb,
		bounds: r,
	}
}

type subFramebuffer struct {
	fb     *Framebuffer
	bounds image.Rectangle
}

func (sf *subFramebuffer) ColorModel() color.Model {
	return sf.fb.ColorModel()
}

func (sf *subFramebuffer) Bounds() image.Rectangle {
	return sf.bounds
}

func (sf *subFramebuffer) At(x, y int) color.Color {
	return sf.fb.At(x, y)
}

func (sf *subFramebuffer) Set(x, y int, c color.Color) {
	if !image.Pt(x, y).In(sf.bounds) {
		return
	}
	sf.fb.Set(x, y, c)
}

func (sf *subFramebuffer) SubImage(r image.Rectangle) draw.Image {
	r = r.Intersect(sf.bounds)
	if r.Empty() {
		return &subFramebuffer{
			fb:     sf.fb,
			bounds: image.Rectangle{},
		}
	}
	return &subFramebuffer{
		fb:     sf.fb,
		bounds: r,
	}
}

func (fb *Framebuffer) RGBA64At(x, y int) color.RGBA64 {
	return fb.ColorModel().Convert(fb.At(x, y)).(color.RGBA64)
}

func (fb *Framebuffer) SetRGBA64(x, y int, c color.RGBA64) {
	fb.Set(x, y, c)
}

func (sf *subFramebuffer) RGBA64At(x, y int) color.RGBA64 {
	return sf.fb.RGBA64At(x, y)
}

func (sf *subFramebuffer) SetRGBA64(x, y int, c color.RGBA64) {
	sf.Set(x, y, c)
}

func (fb *Framebuffer) RGBAAt(x, y int) color.RGBA {
	return color.RGBAModel.Convert(fb.At(x, y)).(color.RGBA)
}

func (fb *Framebuffer) SetRGBA(x, y int, c color.RGBA) {
	fb.Set(x, y, c)
}

func (sf *subFramebuffer) RGBAAt(x, y int) color.RGBA {
	return sf.fb.RGBAAt(x, y)
}

func (sf *subFramebuffer) SetRGBA(x, y int, c color.RGBA) {
	sf.Set(x, y, c)
}

var _ draw.Image = (*Framebuffer)(nil)
var _ draw.Image = (*subFramebuffer)(nil)