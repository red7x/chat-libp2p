package model

import (
	"bytes"
	"encoding/gob"
	"time"
)

type Message struct {
	ID        uint64        `json:"id"`
	Type      string        `json:"type"`
	Payload   []byte        `json:"payload"`
	Reached   bool          `json:"reached"`
	Initiator bool          `json:"initiator"`
	CreatedAt time.Time     `json:"created_at"`
	TTL       time.Duration `json:"ttl,omitempty"`
}

func (m *Message) Encode() (b []byte, err error) {
	buf := bytes.NewBuffer(nil)
	err = gob.NewEncoder(buf).Encode(m)
	if err == nil {
		b = buf.Bytes()
	}
	return
}

func (t *Message) Decode(b []byte) (err error) {
	buf := bytes.NewBuffer(b)
	err = gob.NewDecoder(buf).Decode(t)
	return
}
