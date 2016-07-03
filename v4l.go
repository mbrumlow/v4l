package v4l

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"reflect"
	"syscall"
	"unsafe"
)

var (
	V4L2_PIX_FMT_YUYV  uint32 = 0x56595559
	V4L2_PIX_FMT_RGB32 uint32 = 0x59565955
)

const (
	V4L2_BUF_TYPE_VIDEO_CAPTURE uint32 = 1
	V4L2_MEMORY_USERPTR                = 2

	VIDIOC_S_FMT    uintptr = 0xC0D05605
	VIDIOC_G_FMT            = 0xC0D05604
	VIDIOC_STREAMON         = 0x40045612
	VIDIOC_REQBUFS          = 0xC0145608
	VIDIOC_QBUF             = 0xC058560F
	VIDIOC_DQBUF            = 0xC0585611
)

type v4l2_pix_format struct {
	Type, Width, Height, Pixelformat, Field          uint32
	Bytesperline, Sizeimage, Colorspace, Priv, Flags uint32
	YCBCREnc, Quantization, XferFunc                 uint32
	a                                                uint64
	b                                                uint64
	c                                                uint64
	d                                                uint64
	e                                                uint64
}

type v4l2_requestbuffers struct {
	Count     uint32
	Type      uint32
	Memory    uint32
	Reserved0 uint32
	a         uint64
	b         uint64
	c         uint64
	d         uint64
	e         uint64
}

type v4l2_buffer struct {
	Index, Type, Bytesused, Flags, Field uint32

	// timeval
	TvSec, TvUsec uint64

	// v4l2_timecode
	TcType, TcFlags                                                             uint32
	TcFrames, TcSeconds, TcMinutes, TcHours, TcUser0, TcUser1, TcUser2, TcUser3 uint8

	padding0                    uint32
	Sequence, Memory            uint32
	Userptr                     uint64
	Length, Reserved2, Reserved uint32
	a                           uint64
	b                           uint64
	c                           uint64
	d                           uint64
	e                           uint64
}

type Device struct {
	device string
	fd     int
	width  int
	height int
}

func Open(device string, width, height int) (*Device, error) {

	fd, err := syscall.Open(device, os.O_RDWR|syscall.O_CLOEXEC, 0666)
	if err != nil {
		return nil, fmt.Errorf("Failed to open device: %v", err.Error())
	}

	if err := setFormat(fd, V4L2_PIX_FMT_YUYV, width, height); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("Failed to set format: %v", err.Error())
	}

	if err := setUserptr(fd); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("Failed to set user space ptr: %v", err.Error())
	}

	return &Device{device: device, fd: fd, width: width, height: height}, nil
}

func (dev *Device) Close() {
	syscall.Close(dev.fd)
}

func (dev *Device) GetFrame() (*image.RGBA, error) {

	imageSize := (dev.width * dev.height) * (4 / 2)
	frame := make([]byte, imageSize)

	qbuf := v4l2_buffer{
		Type:    V4L2_BUF_TYPE_VIDEO_CAPTURE,
		Memory:  V4L2_MEMORY_USERPTR,
		Userptr: uint64(toUintptr(frame)),
		Length:  uint32(len(frame)),
	}

	bqbuf := toBytes(qbuf)

	if err := ioctl(dev.fd, VIDIOC_QBUF, toUintptr(bqbuf)); err != nil {
		return nil, fmt.Errorf("Failed to qbuf: %v", err.Error())
	}

	if err := ioctl(dev.fd, VIDIOC_DQBUF, toUintptr(bqbuf)); err != nil {
		return nil, fmt.Errorf("Failed to dqbuf: %v", err.Error())
	}

	r := image.Rect(0, 0, dev.width, dev.height)
	im := image.NewRGBA(r)

	frameToImage(frame, im)

	return im, nil
}

func frameToImage(frame []byte, im *image.RGBA) {

	p := 0
	for i := 0; i < len(frame); i += 4 {

		im.Pix[p+0], im.Pix[p+1], im.Pix[p+2] = color.YCbCrToRGB(
			frame[i+0],
			frame[i+1],
			frame[i+3])
		p += 4

		im.Pix[p+0], im.Pix[p+1], im.Pix[p+2] = color.YCbCrToRGB(
			frame[i+2],
			frame[i+1],
			frame[i+3])
		p += 4

	}

}

func setFormat(fd int, format uint32, width, height int) error {

	f := v4l2_pix_format{
		Type:        uint32(V4L2_BUF_TYPE_VIDEO_CAPTURE),
		Width:       uint32(width),
		Height:      uint32(height),
		Pixelformat: uint32(format),
	}

	b := toBytes(f)

	if err := ioctl(fd, VIDIOC_S_FMT, toUintptr(b)); err != nil {
		return err
	}

	return nil

}

func setUserptr(fd int) error {

	r := v4l2_requestbuffers{
		Count:  1,
		Type:   V4L2_BUF_TYPE_VIDEO_CAPTURE,
		Memory: V4L2_MEMORY_USERPTR,
	}

	b := toBytes(r)

	if err := ioctl(fd, VIDIOC_REQBUFS, toUintptr(b)); err != nil {
		return err
	}

	b2 := toBytes(V4L2_BUF_TYPE_VIDEO_CAPTURE)

	if err := ioctl(fd, VIDIOC_STREAMON, toUintptr(b2)); err != nil {
		return err
	}

	return nil
}

func toBytes(i interface{}) []byte {
	buf := &bytes.Buffer{}
	binary.Write(buf, binary.LittleEndian, i)
	return buf.Bytes()
}

func toUintptr(b []byte) uintptr {
	return (*reflect.SliceHeader)(unsafe.Pointer(&b)).Data
}

func ioctl(fd int, req, arg uintptr) error {
	_, _, e := syscall.RawSyscall(syscall.SYS_IOCTL, uintptr(fd), req, arg)
	if e != 0 {
		log.Printf("IOCTL[%d::%x]: %d -> %v\n", fd, req, e, e)
		return os.NewSyscallError("ioctl", e)
	}
	return nil
}
