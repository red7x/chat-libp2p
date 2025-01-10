package heartbeat

import (
	"chat-libp2p/internal/common"
	proto "chat-libp2p/internal/common/protocol"
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

const Protocol protocol.ID = "/chat-libp2p/heartbeat/1.0.0"

type Heartbeat struct {
	xrmmgr *common.StreamManager
}

func NewHeartbeat(host host.Host) *Heartbeat {
	heartbeat := &Heartbeat{
		xrmmgr: common.NewStreamManager(host, Protocol, nil, nil),
	}
	return heartbeat
}

func (h *Heartbeat) Send(ctx context.Context, peerID peer.ID) (direct bool, err error) {
	xrm, err := h.xrmmgr.GetStream(ctx, peerID)
	if err != nil {
		err = fmt.Errorf("failed to get %s stream: %v", peerID, err)
		return
	}
	dataBytes := proto.NewFrame(proto.FlagAck, nil).Encode()
	if _, err = xrm.WriteWithContext(ctx, dataBytes); err == nil {
		direct = xrm.IsDirect()
		return
	}
	// get stream with no cache and try again
	xrm, err = h.xrmmgr.GetStream(h.xrmmgr.WithNoCache(ctx), peerID)
	if err != nil {
		err = fmt.Errorf("failed to get %s stream: %v", peerID, err)
		return
	}
	if _, err = xrm.WriteWithContext(ctx, dataBytes); err != nil {
		err = fmt.Errorf("failed to write data to %s: %v", peerID, err)
		return
	}
	direct = xrm.IsDirect()
	return
}
