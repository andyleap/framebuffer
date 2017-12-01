package framebuffer

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

type fixedScreenInfo struct {
	Id        [16]byte
	SMemStart uint32
	SMemLen   uint32

	Type         uint32
	TypeAux      uint32
	Visual       uint32
	XPanStep     uint16
	YPanStep     uint16
	YWrapStep    uint16
	pad_0        [2]byte
	LineLength   uint32
	pad_1        [4]byte
	MMIOStart    uint32
	MMIOLen      uint32
	Accel        uint32
	Capabilities uint16
	Reserved     [2]uint16
}

type bitField struct {
	Offset uint32
	Length uint32
	Right  uint32
}

type varScreenInfo struct {
	XRes         uint32
	YRes         uint32
	XResVirtual  uint32
	YResVirtual  uint32
	XOffset      uint32
	YOffset      uint32
	BitsPerPixel uint32
	Grayscale    uint32
	Red          bitField
	Green        bitField
	Blue         bitField
	Alpha        bitField
	NonStd       uint32
	Activate     uint32
	Height       uint32
	Width        uint32
	AccelFlags   uint32

	PixClock    uint32
	LeftMargin  uint32
	RightMargin uint32
	UpperMargin uint32
	LowerMargin uint32
	HSyncLen    uint32
	VSyncLen    uint32
	Sync        uint32
	VMode       uint32
	Rotate      uint32
	Colorspace  uint32
}

const (
	getVarScreenInfo   uintptr = 0x4600
	putVarScreenInfo   uintptr = 0x4601
	getFixedScreenInfo uintptr = 0x4602
	panDisplay         uintptr = 0x4606

	protocolRead  int = 0x01
	protocolWrite int = 0x02
	mapShared     int = 0x01
)

type Framebuffer struct {
	dev     *os.File
	finfo   fixedScreenInfo
	vinfo   varScreenInfo
	swapped bool

	data     []byte
	restData []byte

	curBuf []byte
	FramebufferImage
}

func New(dev string) (*Framebuffer, error) {
	f, err := os.OpenFile(dev, os.O_RDWR, os.ModeDevice)
	if err != nil {
		return nil, err
	}

	fb := &Framebuffer{
		dev: f,
	}

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), getVarScreenInfo, uintptr(unsafe.Pointer(&fb.vinfo)))
	if errno != 0 {
		f.Close()
		return nil, fmt.Errorf("error getting vinfo: %s", errno.Error())
	}

	fb.vinfo.YResVirtual = fb.vinfo.YRes * 2

	_, _, errno = unix.Syscall(unix.SYS_IOCTL, f.Fd(), putVarScreenInfo, uintptr(unsafe.Pointer(&fb.vinfo)))
	if errno != 0 {
		f.Close()
		return nil, fmt.Errorf("error getting vinfo: %s", errno.Error())
	}

	_, _, errno = unix.Syscall(unix.SYS_IOCTL, f.Fd(), getFixedScreenInfo, uintptr(unsafe.Pointer(&fb.finfo)))
	if errno != 0 {
		f.Close()
		return nil, fmt.Errorf("error getting finfo: %s", errno.Error())
	}

	memlen := fb.finfo.SMemLen
	fb.data, err = unix.Mmap(int(f.Fd()), 0, int(memlen), protocolRead|protocolWrite, mapShared)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("error mapping file: %s", err.Error())
	}
	fmt.Print("\x1b[?25l\x1b[?1c")


	fb.restData = make([]byte, len(fb.data))
	copy(fb.restData, fb.data)
	fb.fb = fb
	fb.FramebufferImage.rect = image.Rect(0, 0, int(fb.vinfo.XRes), int(fb.vinfo.YRes))
	fb.FramebufferImage.stride = int(fb.finfo.LineLength)
	fb.Swap()
	return fb, nil
}

func (fb *Framebuffer) Close() {
	if fb.swapped {
		fb.Swap()
	}
	copy(fb.data, fb.restData)
	unix.Munmap(fb.data)
	fb.dev.Close()
	fmt.Print("\x1b[?25h\x1b[?0c")
}

func (fb *Framebuffer) Swap() {
	fb.curBuf = fb.data[fb.finfo.LineLength*fb.vinfo.YOffset:]
	fb.vinfo.YOffset = fb.vinfo.YRes
	if fb.swapped {
		fb.vinfo.YOffset = 0
	}
	fb.swapped = !fb.swapped
	unix.Syscall(unix.SYS_IOCTL, fb.dev.Fd(), panDisplay, uintptr(unsafe.Pointer(&fb.vinfo)))
}

func (fb *Framebuffer) Clear() {
	for l1 := 0; l1 < int(fb.vinfo.YRes*fb.finfo.LineLength); l1++ {
		fb.curBuf[l1] = 0
	}
}

type FramebufferImage struct {
	fb     *Framebuffer
	stride int
	rect   image.Rectangle
}

type FBRGBA struct {
	B, G, R, A uint8
}

func (c FBRGBA) RGBA() (r, g, b, a uint32) {
	return uint32(c.R), uint32(c.G), uint32(c.B), uint32(c.A)
}

var FBRGBAModel = color.ModelFunc(convertToFBRGBA)

func convertToFBRGBA(c color.Color) color.Color {
	if fbc, ok := c.(FBRGBA); ok {
		return fbc
	}
	r, g, b, a := c.RGBA()
	return FBRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(a)}
}

func (fb *FramebufferImage) ColorModel() color.Model {
	return FBRGBAModel
}

func (fb *FramebufferImage) Bounds() image.Rectangle {
	return fb.rect
}

func (fb *FramebufferImage) At(x, y int) color.Color {
	if !(image.Point{x, y}).In(fb.rect) {
		return color.Transparent
	}
	start := x*4 + y*fb.stride
	return FBRGBA{
		B: fb.fb.curBuf[start],
		G: fb.fb.curBuf[start+1],
		R: fb.fb.curBuf[start+2],
		A: fb.fb.curBuf[start+3],
	}

}

func (fb *FramebufferImage) Set(x, y int, c color.Color) {
	if !(image.Point{x, y}).In(fb.rect) {
		return
	}
	start := x*4 + y*fb.stride
	fbc := convertToFBRGBA(c).(FBRGBA)
	fb.fb.curBuf[start] = fbc.B
	fb.fb.curBuf[start+1] = fbc.G
	fb.fb.curBuf[start+2] = fbc.R
	fb.fb.curBuf[start+3] = fbc.A
}

func (fb *FramebufferImage) SubImage(r image.Rectangle) image.Image {
	r = r.Intersect(fb.rect)
	if r.Empty() {
		return nil
	}
	return &FramebufferImage{
		fb:     fb.fb,
		stride: fb.stride,
		rect:   r,
	}
}
