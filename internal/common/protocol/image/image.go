package image

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

const Protocol protocol.ID = "/chat-libp2p/image/1.0.0"

var (
	ImageRecvBufSize int = 1000
)

type Image struct {
	xrmmgr   *common.StreamManager
	result   map[uint64]chan proto.Response
	resultMU *sync.Mutex
	receiver chan proto.PeerRequest
}

func NewImage(host host.Host) *Image {
	image := &Image{
		xrmmgr:   nil,
		result:   make(map[uint64]chan proto.Response),
		resultMU: &sync.Mutex{},
		receiver: make(chan proto.PeerRequest, ImageRecvBufSize),
	}
	image.xrmmgr = common.NewStreamManager(host, Protocol, image.handleMeta, image.handleData)
	return image
}

func (i *Image) handleMeta(peerID peer.ID, payload []byte) {
	switch action := i.parse(payload).(type) {
	case proto.Response:
		i.resultMU.Lock()
		result, ok := i.result[action.ID()]
		i.resultMU.Unlock()
		if ok {
			select {
			case result <- action:
			default:
			}
		}
	case proto.Request:
		select {
		case i.receiver <- proto.PeerRequest{ID: peerID, Request: action}:
		default:
			logger.Warn("image receiver buffer overflow")
		}
	}
}

func (i *Image) handleData(peerID peer.ID, payload []byte) {
	switch action := i.parse(payload).(type) {
	case proto.Request:
		select {
		case i.receiver <- proto.PeerRequest{ID: peerID, Request: action}:
		default:
			logger.Warn("image receiver buffer overflow")
		}
	}
}

func (i *Image) Request(ctx context.Context, peerID peer.ID, req proto.Request) (resp proto.Response, err error) {
	txid := req.ID()
	result := make(chan proto.Response, 1)

	i.resultMU.Lock()
	i.result[txid] = result
	i.resultMU.Unlock()

	defer func() {
		i.resultMU.Lock()
		delete(i.result, txid)
		i.resultMU.Unlock()
	}()

	if err = i.Send(ctx, peerID, req); err != nil {
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

func (i *Image) Send(ctx context.Context, peerID peer.ID, action proto.Action) (err error) {
	xrm, err := i.xrmmgr.GetStream(ctx, peerID)
	if err != nil {
		err = fmt.Errorf("failed to get %s stream: %v", peerID, err)
		return
	}
	frameBytes := action.FrameBytes()
	if _, err = xrm.WriteWithContext(ctx, frameBytes); err == nil {
		return
	}
	// get stream with no cache and try again
	xrm, err = i.xrmmgr.GetStream(i.xrmmgr.WithNoCache(ctx), peerID)
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

func (i *Image) Receiver(ctx context.Context) chan proto.PeerRequest {
	return i.receiver
}
