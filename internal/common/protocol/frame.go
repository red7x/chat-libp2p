package protocol

import "encoding/binary"

type Flag uint8

const (
	FlagAck Flag = iota
	FlagMeta
	FlagData
)

type Frame struct {
	Flag    Flag
	Payload []byte
}

func NewFrame(flag Flag, payload []byte) *Frame {
	frame := &Frame{
		Flag:    flag,
		Payload: payload,
	}
	return frame
}

func (f *Frame) Encode() (buf []byte) {
	psize := uint64(len(f.Payload))
	switch {
	case psize <= 0x0f:
		buf = make([]byte, psize+1)
		buf[0] = byte(f.Flag<<5) | byte(psize)
		copy(buf[1:], f.Payload)
	case psize <= 0xff:
		buf = make([]byte, psize+2)
		buf[0] = byte(f.Flag<<5) | 0x10
		buf[1] = byte(psize)
		copy(buf[2:], f.Payload)
	case psize <= 0xffff:
		buf = make([]byte, psize+3)
		buf[0] = byte(f.Flag<<5) | 0x11
		binary.BigEndian.PutUint16(buf[1:], uint16(psize))
		copy(buf[3:], f.Payload)
	case psize <= 0xffffffff:
		buf = make([]byte, psize+5)
		buf[0] = byte(f.Flag<<5) | 0x12
		binary.BigEndian.PutUint32(buf[1:], uint32(psize))
		copy(buf[5:], f.Payload)
	default:
		buf = make([]byte, psize+9)
		buf[0] = byte(f.Flag<<5) | 0x13
		binary.BigEndian.PutUint64(buf[1:], uint64(psize))
		copy(buf[9:], f.Payload)
	}
	return
}

func (f *Frame) Decode(buf []byte) {
	f.Flag = Flag(buf[0] >> 5)
	size := buf[0] & 0x1f
	switch size {
	case 0x10:
		f.Payload = make([]byte, buf[1])
		copy(f.Payload, buf[2:])
	case 0x11:
		f.Payload = make([]byte, binary.BigEndian.Uint16(buf[1:]))
		copy(f.Payload, buf[3:])
	case 0x12:
		f.Payload = make([]byte, binary.BigEndian.Uint32(buf[1:]))
		copy(f.Payload, buf[5:])
	case 0x13:
		f.Payload = make([]byte, binary.BigEndian.Uint64(buf[1:]))
		copy(f.Payload, buf[9:])
	default:
		f.Payload = make([]byte, size)
		copy(f.Payload, buf[1:])
	}
}
