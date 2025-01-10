package model

import (
	"bytes"
	"encoding/gob"
)

type Peers []*Peer

func (p Peers) Len() int      { return len(p) }
func (p Peers) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p Peers) Less(i, j int) bool {
	if p[i].Pin == p[j].Pin {
		if p[i].LastMessage == nil && p[j].LastMessage == nil {
			return p[i].ID < p[j].ID
		} else if p[i].LastMessage != nil && p[j].LastMessage != nil {
			return p[i].LastMessage.CreatedAt.After(p[j].LastMessage.CreatedAt)
		} else if p[i].LastMessage != nil {
			return true
		} else {
			return false
		}
	} else if p[i].Pin {
		return true
	} else {
		return false
	}
}

type Peer struct {
	ID          string   `json:"id"`
	EVMAddr     string   `json:"evm_addr"`
	Remark      string   `json:"remark"`
	Pin         bool     `json:"pin"`
	LastMessage *Message `json:"last_message" gob:"-"`
}

func (p *Peer) Encode() (b []byte, err error) {
	buf := bytes.NewBuffer(nil)
	err = gob.NewEncoder(buf).Encode(p)
	if err == nil {
		b = buf.Bytes()
	}
	return
}

func (p *Peer) Decode(b []byte) (err error) {
	buf := bytes.NewBuffer(b)
	err = gob.NewDecoder(buf).Decode(p)
	return
}
