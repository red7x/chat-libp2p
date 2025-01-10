package image

import (
	proto "chat-libp2p/internal/common/protocol"
	"encoding/binary"
	"path/filepath"
	"strings"
)

const (
	flagSyncHash uint8 = iota
	flagSyncBytes
	flagExistence
)

const (
	typeRequest uint8 = iota
	typeResponse
)

func (i *Image) parse(payload []byte) (action proto.Action) {
	switch payload[0] {
	case flagSyncHash:
		if payload[1] == typeRequest {
			action = &ReqSyncHash{}
			action.Decode(payload)
		} else if payload[1] == typeResponse {
			action = &RespSyncHash{}
			action.Decode(payload)
		}
	case flagSyncBytes:
		if payload[1] == typeRequest {
			action = &ReqSyncBytes{}
			action.Decode(payload)
		} else if payload[1] == typeResponse {
			action = &RespSyncBytes{}
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

type ReqSyncHash struct {
	id   uint64
	ext  string
	hash string
}

func NewReqSyncHash(id uint64, filebase string) *ReqSyncHash {
	req := &ReqSyncHash{id: id}
	req.ext = filepath.Ext(filebase)
	req.hash = strings.TrimSuffix(filebase, req.ext)
	return req
}

func (r *ReqSyncHash) ID() uint64 {
	return r.id
}

func (r *ReqSyncHash) Decode(payload []byte) {
	r.id = binary.BigEndian.Uint64(payload[2:])
	filebase := string(payload[10:])
	r.ext = filepath.Ext(filebase)
	r.hash = strings.TrimSuffix(filebase, r.ext)
}

func (r *ReqSyncHash) FrameBytes() []byte {
	payload := make([]byte, 10)
	payload[0] = flagSyncHash
	payload[1] = typeRequest
	binary.BigEndian.PutUint64(payload[2:], r.id)
	payload = append(payload, r.hash...)
	payload = append(payload, r.ext...)
	return proto.NewFrame(proto.FlagMeta, payload).Encode()
}

func (r *ReqSyncHash) Filebase() string {
	return r.hash + r.ext
}

func (r *ReqSyncHash) Ext() string {
	return r.ext
}

func (r *ReqSyncHash) Hash() string {
	return r.hash
}

type RespSyncHash struct {
	id     uint64
	ok     bool
	subdir string
}

func NewRespSyncHash(id uint64, ok bool, subdir string) *RespSyncHash {
	return &RespSyncHash{id, ok, subdir}
}

func (r *RespSyncHash) ID() uint64 {
	return r.id
}

func (r *RespSyncHash) Decode(payload []byte) {
	r.id = binary.BigEndian.Uint64(payload[2:])
	if payload[10] == 1 {
		r.ok = true
	}
	r.subdir = string(payload[11:])
}

func (r *RespSyncHash) FrameBytes() []byte {
	payload := make([]byte, 11)
	payload[0] = flagSyncHash
	payload[1] = typeResponse
	binary.BigEndian.PutUint64(payload[2:], r.id)
	if r.ok {
		payload[10] = 1
	}
	payload = append(payload, r.subdir...)
	return proto.NewFrame(proto.FlagMeta, payload).Encode()
}

func (r *RespSyncHash) OK() bool {
	return r.ok
}

func (r *RespSyncHash) Subdir() string {
	return r.subdir
}

type ReqSyncBytes struct {
	id      uint64
	subdir  string
	payload []byte
}

func NewReqSyncBytes(id uint64, subdir string, payload []byte) *ReqSyncBytes {
	return &ReqSyncBytes{id, subdir, payload}
}

func (r *ReqSyncBytes) ID() uint64 {
	return r.id
}

func (r *ReqSyncBytes) Decode(payload []byte) {
	r.id = binary.BigEndian.Uint64(payload[2:])
	s := payload[10] + 11
	r.subdir = string(payload[11:s])
	r.payload = make([]byte, len(payload[s:]))
	copy(r.payload, payload[s:])
}

func (r *ReqSyncBytes) FrameBytes() []byte {
	payload := make([]byte, 11)
	payload[0] = flagSyncBytes
	payload[1] = typeRequest
	binary.BigEndian.PutUint64(payload[2:], r.id)
	payload[10] = byte(len(r.subdir))
	payload = append(payload, r.subdir...)
	payload = append(payload, r.payload...)
	return proto.NewFrame(proto.FlagData, payload).Encode()
}

func (r *ReqSyncBytes) Subdir() string {
	return r.subdir
}

func (r *ReqSyncBytes) Payload() []byte {
	payload := make([]byte, len(r.payload))
	copy(payload, r.payload)
	return payload
}

type RespSyncBytes struct {
	id uint64
	ok bool
}

func NewRespSyncBytes(id uint64, ok bool) *RespSyncBytes {
	resp := &RespSyncBytes{
		id: id,
		ok: ok,
	}
	return resp
}

func (r *RespSyncBytes) ID() uint64 {
	return r.id
}

func (r *RespSyncBytes) Decode(payload []byte) {
	r.id = binary.BigEndian.Uint64(payload[2:])
	if payload[10] == 1 {
		r.ok = true
	}
}

func (r *RespSyncBytes) FrameBytes() []byte {
	payload := make([]byte, 11)
	payload[0] = flagSyncBytes
	payload[1] = typeResponse
	binary.BigEndian.PutUint64(payload[2:], r.id)
	if r.ok {
		payload[10] = 1
	}
	return proto.NewFrame(proto.FlagMeta, payload).Encode()
}

func (r *RespSyncBytes) OK() bool {
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
