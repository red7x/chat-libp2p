package common

import (
	proto "chat-libp2p/internal/common/protocol"
	"chat-libp2p/pkg/logger"
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	ma "github.com/multiformats/go-multiaddr"
)

type ctxKey struct{}

var (
	noCache   ctxKey
	StreamTTL time.Duration = time.Second * 30
)

type Stream struct {
	network.Stream
}

func NewStream(s network.Stream) *Stream {
	xrm := &Stream{s}
	return xrm
}

func (s *Stream) LocalAddr() net.Addr  { return nil }
func (s *Stream) RemoteAddr() net.Addr { return nil }

func (s *Stream) IsDirect() bool {
	_, err := s.Conn().RemoteMultiaddr().ValueForProtocol(ma.P_CIRCUIT)
	return err != nil
}

func (s *Stream) WriteWithContext(ctx context.Context, data []byte) (n int, err error) {
	var exp *time.Duration
	if deadline, ok := ctx.Deadline(); ok {
		d := time.Until(deadline)
		if d < 0 {
			d = 0
		}
		exp = &d
	}
	for i, size := 0, len(data); i < size; {
		end := i + 4096
		if end > size {
			end = size
		}
		now := time.Now()
		if exp != nil {
			s.SetWriteDeadline(now.Add(*exp))
		}
		s.SetReadDeadline(now.Add(StreamTTL))
		num, er := s.Stream.Write(data[i:end])
		if er != nil {
			s.Close()
			err = fmt.Errorf("failed to write data to stream: %v", er)
			return
		}
		i = end
		n += num
	}
	return
}

type StreamManager struct {
	host      host.Host
	proto     protocol.ID
	metaFunc  func(peerID peer.ID, payload []byte)
	dataFunc  func(peerID peer.ID, payload []byte)
	streams   map[peer.ID]*Stream
	streamsMU *sync.Mutex
}

func NewStreamManager(
	host host.Host,
	proto protocol.ID,
	metaFunc func(peerID peer.ID, payload []byte),
	dataFunc func(peerID peer.ID, payload []byte),
) *StreamManager {
	xrmmgr := &StreamManager{
		host:      host,
		proto:     proto,
		metaFunc:  metaFunc,
		dataFunc:  dataFunc,
		streams:   make(map[peer.ID]*Stream),
		streamsMU: &sync.Mutex{},
	}
	xrmmgr.host.SetStreamHandler(xrmmgr.proto, xrmmgr.handle)
	xrmmgr.host.Network().Notify(xrmmgr)
	return xrmmgr
}

func (s *StreamManager) readLoop(xrm *Stream) {
	remotePeerId := xrm.Conn().RemotePeer()
	defer func() {
		s.streamsMU.Lock()
		if s.streams[remotePeerId] == xrm {
			delete(s.streams, remotePeerId)
		}
		s.streamsMU.Unlock()
		xrm.Close()
	}()

	frame := &proto.Frame{}
	reader := proto.NewReader(xrm, StreamTTL)
	for {
		if err := reader.Read(frame); err != nil {
			logger.Error("failed to read frame: %p %v", xrm, err)
			return
		}
		switch frame.Flag {
		case proto.FlagAck:
		case proto.FlagMeta:
			s.metaFunc(remotePeerId, frame.Payload)
		case proto.FlagData:
			s.dataFunc(remotePeerId, frame.Payload)
		default:
		}
	}
}

func (s *StreamManager) handle(ns network.Stream) {
	xrm := NewStream(ns)
	rPeerId := xrm.Conn().RemotePeer()

	s.streamsMU.Lock()
	defer s.streamsMU.Unlock()

	if old, ok := s.streams[rPeerId]; !ok {
		s.streams[rPeerId] = xrm
	} else if xrm.IsDirect() {
		s.streams[rPeerId] = xrm
	} else if xrm.Stat().Direction == network.DirInbound {
		if old.Stat().Direction == network.DirInbound {
			s.streams[rPeerId] = xrm
		} else if old.Stat().Direction == network.DirOutbound && rPeerId < xrm.Conn().LocalPeer() {
			s.streams[rPeerId] = xrm
		}
	} else if xrm.Stat().Direction == network.DirOutbound {
		if old.Stat().Direction == network.DirOutbound {
			s.streams[rPeerId] = xrm
		} else if old.Stat().Direction == network.DirInbound && xrm.Conn().LocalPeer() < rPeerId {
			s.streams[rPeerId] = xrm
		}
	}

	go s.readLoop(xrm)
}

func (s *StreamManager) WithNoCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, noCache, true)
}

func (s *StreamManager) GetStream(ctx context.Context, peerID peer.ID) (xrm *Stream, err error) {
	s.streamsMU.Lock()
	xrm, ok := s.streams[peerID]
	s.streamsMU.Unlock()

	if ok && ctx.Value(noCache) == nil {
		select {
		case <-ctx.Done():
			err = ctx.Err()
		default:
		}
		return
	}

	ns, err := s.host.NewStream(ctx, peerID, s.proto)
	if err != nil {
		s.host.Peerstore().RemovePeer(peerID)
		s.host.Peerstore().ClearAddrs(peerID)
		err = fmt.Errorf("failed to new stream to %s: %v", peerID, err)
		return
	}
	s.handle(ns)

	s.streamsMU.Lock()
	xrm = s.streams[peerID]
	s.streamsMU.Unlock()

	return
}

func (s *StreamManager) Listen(n network.Network, addr ma.Multiaddr)      {}
func (s *StreamManager) ListenClose(n network.Network, addr ma.Multiaddr) {}
func (s *StreamManager) Connected(n network.Network, c network.Conn) {
	rPeerID := c.RemotePeer()

	s.streamsMU.Lock()
	_, ok := s.streams[rPeerID]
	s.streamsMU.Unlock()

	_, err := c.RemoteMultiaddr().ValueForProtocol(ma.P_CIRCUIT)
	direct := err != nil

	if ok && direct && c.Stat().Direction == network.DirOutbound {
		s.GetStream(s.WithNoCache(context.Background()), rPeerID)
	}
}
func (s *StreamManager) Disconnected(n network.Network, c network.Conn) {}
