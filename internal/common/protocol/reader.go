package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

type Reader struct {
	conn    net.Conn
	timeout time.Duration
	header  []byte
}

func NewReader(conn net.Conn, timeout time.Duration) *Reader {
	reader := &Reader{
		conn:    conn,
		timeout: timeout,
		header:  make([]byte, 9),
	}
	return reader
}

func (r *Reader) Read(frame *Frame) (err error) {
	r.conn.SetReadDeadline(time.Now().Add(r.timeout))
	if _, err = io.ReadFull(r.conn, r.header[:1]); err != nil {
		err = fmt.Errorf("failed to read meta byte: %v", err)
		return
	}
	frame.Flag = Flag(r.header[0] >> 5)
	size := r.header[0] & 0x1f
	switch size {
	case 0x10:
		if _, err = io.ReadFull(r.conn, r.header[1:2]); err != nil {
			err = fmt.Errorf("failed to read size byte: %v", err)
			return
		}
		frame.Payload = make([]byte, r.header[1])
	case 0x11:
		if _, err = io.ReadFull(r.conn, r.header[1:3]); err != nil {
			err = fmt.Errorf("failed to read size byte: %v", err)
			return
		}
		frame.Payload = make([]byte, binary.BigEndian.Uint16(r.header[1:]))
	case 0x12:
		if _, err = io.ReadFull(r.conn, r.header[1:5]); err != nil {
			err = fmt.Errorf("failed to read size byte: %v", err)
			return
		}
		frame.Payload = make([]byte, binary.BigEndian.Uint32(r.header[1:]))
	case 0x13:
		if _, err = io.ReadFull(r.conn, r.header[1:9]); err != nil {
			err = fmt.Errorf("failed to read size byte: %v", err)
			return
		}
		frame.Payload = make([]byte, binary.BigEndian.Uint64(r.header[1:]))
	default:
		frame.Payload = make([]byte, size)
	}
	for i, size := 0, len(frame.Payload); i < size; {
		end := i + 4096
		if end > size {
			end = size
		}
		r.conn.SetReadDeadline(time.Now().Add(r.timeout))
		if _, err = io.ReadFull(r.conn, frame.Payload[i:end]); err != nil {
			err = fmt.Errorf("failed to read payload byte: %v", err)
			return
		}
		i = end
	}
	return
}
