package text

import (
	proto "chat-libp2p/internal/common/protocol"
	"encoding/binary"
	"time"
)

const (
	flagSyncText uint8 = iota
	flagExistence
)

const (
	typeRequest uint8 = iota
	typeResponse
)

func (t *Text) parse(payload []byte) (action proto.Action) {
	switch payload[0] {
	case flagSyncText:
		if payload[1] == typeRequest {
			action = &ReqSyncText{}
			action.Decode(payload)
		} else if payload[1] == typeResponse {
			action = &RespSyncText{}
			action.Decode(payload)
		}
	case flagExistence:
		if payload[1] == typeRequest {
			action = &ReqExistence{}
			action.Decode(payload)
		} else if payload[1] == typeResponse {
			action = &RespExistence{}
			action.Decode(payload)
		}
	}
	return
}

type ReqSyncText struct {
	id      uint64
	ttl     uint64
	content string
}

func NewReqSyncText(id uint64, ttl time.Duration, content string) *ReqSyncText {
	return &ReqSyncText{id, uint64(ttl), content}
}

func (r *ReqSyncText) ID() uint64 {
	return r.id
}

func (r *ReqSyncText) Decode(payload []byte) {
	r.id = binary.BigEndian.Uint64(payload[2:])
	if payload[10] == 0 {
		r.ttl = 0
		r.content = string(payload[11:])
	} else {
		r.ttl = binary.BigEndian.Uint64(payload[11:])
		r.content = string(payload[19:])
	}
}

func (r *ReqSyncText) FrameBytes() []byte {
	payload := make([]byte, 11)
	payload[0] = flagSyncText
	payload[1] = typeRequest
	binary.BigEndian.PutUint64(payload[2:], r.id)
	if r.ttl > 0 {
		payload[10] = 1
		payload = append(payload, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.BigEndian.PutUint64(payload[11:], r.ttl)
	}
	payload = append(payload, r.content...)
	return proto.NewFrame(proto.FlagData, payload).Encode()
}

func (r *ReqSyncText) TTL() time.Duration {
	return time.Duration(r.ttl)
}

func (r *ReqSyncText) ContentBytes() []byte {
	return []byte(r.content)
}

type RespSyncText struct {
	id uint64
	ok bool
}

func NewRespSyncText(id uint64, ok bool) *RespSyncText {
	return &RespSyncText{id, ok}
}

func (r *RespSyncText) ID() uint64 {
	return r.id
}

func (r *RespSyncText) Decode(payload []byte) {
	r.id = binary.BigEndian.Uint64(payload[2:])
	if payload[10] == 1 {
		r.ok = true
	}
}

func (r *RespSyncText) FrameBytes() []byte {
	payload := make([]byte, 11)
	payload[0] = flagSyncText
	payload[1] = typeResponse
	binary.BigEndian.PutUint64(payload[2:], r.id)
	if r.ok {
		payload[10] = 1
	}
	return proto.NewFrame(proto.FlagMeta, payload).Encode()
}

func (r *RespSyncText) OK() bool {
	return r.ok
}

type ReqExistence struct {
	id uint64
}

func NewReqExistence(id uint64) *ReqExistence {
	return &ReqExistence{id}
}

func (r *ReqExistence) ID() uint64 {
	return r.id
}

func (r *ReqExistence) Decode(payload []byte) {
	r.id = binary.BigEndian.Uint64(payload[2:])
}

func (r *ReqExistence) FrameBytes() []byte {
	payload := make([]byte, 10)
	payload[0] = flagExistence
	payload[1] = typeRequest
	binary.BigEndian.PutUint64(payload[2:], r.id)
	return proto.NewFrame(proto.FlagMeta, payload).Encode()
}

type RespExistence struct {
	id    uint64
	exist bool
}

func NewRespExistence(id uint64, exist bool) *RespExistence {
	return &RespExistence{id, exist}
}

func (r *RespExistence) ID() uint64 {
	return r.id
}

func (r *RespExistence) Decode(payload []byte) {
	r.id = binary.BigEndian.Uint64(payload[2:])
	if payload[10] == 1 {
		r.exist = true
	}
}

func (r *RespExistence) FrameBytes() []byte {
	payload := make([]byte, 11)
	payload[0] = flagExistence
	payload[1] = typeResponse
	binary.BigEndian.PutUint64(payload[2:], r.id)
	if r.exist {
		payload[10] = 1
	}
	return proto.NewFrame(proto.FlagMeta, payload).Encode()
}

func (r *RespExistence) OK() bool {
	return r.exist
}
