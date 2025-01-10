package text

import (
	"chat-libp2p/internal/common"
	proto "chat-libp2p/internal/common/protocol"
	"chat-libp2p/pkg/logger"
	"context"
	"fmt"
	"sync"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

const Protocol protocol.ID = "/chat-libp2p/text/1.0.0"

var (
	TextRecvBufSize int = 1000
)

type Text struct {
	xrmmgr   *common.StreamManager
	result   map[uint64]chan proto.Response
	resultMU *sync.Mutex
	receiver chan proto.PeerRequest
}

func NewText(host host.Host) *Text {
	text := &Text{
		xrmmgr:   nil,
		result:   make(map[uint64]chan proto.Response),
		resultMU: &sync.Mutex{},
		receiver: make(chan proto.PeerRequest, TextRecvBufSize),
	}
	text.xrmmgr = common.NewStreamManager(host, Protocol, text.handleMeta, text.handleData)
	return text
}

func (t *Text) handleMeta(peerID peer.ID, payload []byte) {
	switch action := t.parse(payload).(type) {
	case proto.Response:
		t.resultMU.Lock()
		result, ok := t.result[action.ID()]
		t.resultMU.Unlock()
		if ok {
			select {
			case result <- action:
			default:
			}
		}
	case proto.Request:
		select {
		case t.receiver <- proto.PeerRequest{ID: peerID, Request: action}:
		default:
			logger.Warn("text receiver buffer overflow")
		}
	}
}

func (t *Text) handleData(peerID peer.ID, payload []byte) {
	switch action := t.parse(payload).(type) {
	case proto.Request:
		select {
		case t.receiver <- proto.PeerRequest{ID: peerID, Request: action}:
		default:
			logger.Warn("text receiver buffer overflow")
		}
	}
}

func (t *Text) Request(ctx context.Context, peerID peer.ID, req proto.Request) (resp proto.Response, err error) {
	txid := req.ID()
	result := make(chan proto.Response, 1)

	t.resultMU.Lock()
	t.result[txid] = result
	t.resultMU.Unlock()

	defer func() {
		t.resultMU.Lock()
		delete(t.result, txid)
		t.resultMU.Unlock()
	}()

	if err = t.Send(ctx, peerID, req); err != nil {
		err = fmt.Errorf("failed to send request: %v", err)
		return
	}

	select {
	case resp = <-result:
	case <-ctx.Done():
		err = ctx.Err()
	}
	return
}

func (t *Text) Send(ctx context.Context, peerID peer.ID, action proto.Action) (err error) {
	xrm, err := t.xrmmgr.GetStream(ctx, peerID)
	if err != nil {
		err = fmt.Errorf("failed to get %s stream: %v", peerID, err)
		return
	}
	frameBytes := action.FrameBytes()
	if _, err = xrm.WriteWithContext(ctx, frameBytes); err == nil {
		return
	}
	// get stream with no cache and try again
	xrm, err = t.xrmmgr.GetStream(t.xrmmgr.WithNoCache(ctx), peerID)
	if err != nil {
		err = fmt.Errorf("failed to get %s stream: %v", peerID, err)
		return
	}
	if _, err = xrm.WriteWithContext(ctx, frameBytes); err != nil {
		err = fmt.Errorf("failed to write data to %s: %v", peerID, err)
		return
	}
	return
}

func (t *Text) Receiver(ctx context.Context) chan proto.PeerRequest {
	return t.receiver
}
