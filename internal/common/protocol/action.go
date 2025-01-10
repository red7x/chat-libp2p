package protocol

import "github.com/libp2p/go-libp2p/core/peer"

type Action interface {
	ID() uint64
	Decode(payload []byte)
	FrameBytes() (frameBytes []byte)
}

type Request interface {
	Action
}

type Response interface {
	Action
	OK() bool
}

type PeerRequest struct {
	peer.ID
	Request
}
