package identify

import (
	"chat-libp2p/internal/common"
	proto "chat-libp2p/internal/common/protocol"
	"chat-libp2p/pkg/logger"
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

const Protocol protocol.ID = "/chat-libp2p/identify/1.0.0"

var (
	IdentifyRespTimeout time.Duration = time.Second * 3
)

type Identify struct {
	evmAddr  string
	xrmmgr   *common.StreamManager
	result   map[uint16]chan string
	resultID uint16
	resultMU *sync.Mutex
}

func NewIdentify(host host.Host, evmAddr string) *Identify {
	identify := &Identify{
		evmAddr:  evmAddr,
		xrmmgr:   nil,
		result:   make(map[uint16]chan string),
		resultID: 0,
		resultMU: &sync.Mutex{},
	}
	identify.xrmmgr = common.NewStreamManager(host, Protocol, identify.handleMeta, identify.handleData)
	return identify
}

func (i *Identify) handleMeta(peerID peer.ID, payload []byte) {
	payload = append(payload, i.evmAddr...)
	dataBytes := proto.NewFrame(proto.FlagData, payload).Encode()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), IdentifyRespTimeout)
		defer cancel()

		xrm, err := i.xrmmgr.GetStream(ctx, peerID)
		if err != nil {
			logger.Error("failed to get %s stream: %v", peerID, err)
			return
		}
		if _, err = xrm.WriteWithContext(ctx, dataBytes); err == nil {
			return
		}
		// get stream with no cache and try again
		xrm, err = i.xrmmgr.GetStream(i.xrmmgr.WithNoCache(ctx), peerID)
		if err != nil {
			logger.Error("failed to get %s stream: %v", peerID, err)
			return
		}
		if _, err = xrm.WriteWithContext(ctx, dataBytes); err != nil {
			logger.Error("failed to write response data to %s: %v", peerID, err)
			return
		}
	}()
}

func (i *Identify) handleData(peerID peer.ID, payload []byte) {
	id := binary.BigEndian.Uint16(payload)
	evm := string(payload[2:])

	i.resultMU.Lock()
	result, ok := i.result[id]
	i.resultMU.Unlock()
	if ok {
		select {
		case result <- evm:
		default:
		}
	}
}

func (i *Identify) QueryEVM(ctx context.Context, peerID peer.ID) (evm string, err error) {
	i.resultMU.Lock()
	id := i.resultID
	result := make(chan string, 1)
	i.result[id] = result
	i.resultID++
	i.resultMU.Unlock()

	defer func() {
		i.resultMU.Lock()
		delete(i.result, id)
		i.resultMU.Unlock()
	}()

	reqFrame := proto.NewFrame(proto.FlagMeta, make([]byte, 2))
	binary.BigEndian.PutUint16(reqFrame.Payload, id)
	reqBytes := reqFrame.Encode()

	switch {
	default:
		xrm, e := i.xrmmgr.GetStream(ctx, peerID)
		if err != nil {
			err = fmt.Errorf("failed to get %s stream: %v", peerID, e)
			return
		}
		if _, err = xrm.WriteWithContext(ctx, reqBytes); err == nil {
			break
		}
		// get stream with no cache and try again
		xrm, err = i.xrmmgr.GetStream(i.xrmmgr.WithNoCache(ctx), peerID)
		if err != nil {
			err = fmt.Errorf("failed to get %s stream: %v", peerID, err)
			return
		}
		if _, err = xrm.WriteWithContext(ctx, reqBytes); err != nil {
			err = fmt.Errorf("failed to write request data to %s: %v", peerID, err)
			return
		}
	}

	select {
	case evm = <-result:
	case <-ctx.Done():
		err = ctx.Err()
	}
	return
}
