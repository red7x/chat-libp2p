package service

import (
	"chat-libp2p/internal/common/protocol/heartbeat"
	"chat-libp2p/pkg/logger"
	"context"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

type ProbeStatus uint8

const (
	ProbeStatusOff ProbeStatus = iota
	ProbeStatusRelay
	ProbeStatusDirect
)

var (
	ProbeInterval time.Duration = time.Second * 2
)

type Probe struct {
	ctx        context.Context
	heartbeat  *heartbeat.Heartbeat
	svcPeer    *Peer
	runMU      *sync.Mutex
	stopFunc   context.CancelFunc
	stopFuncMU *sync.Mutex
}

func NewProbe(ctx context.Context, host host.Host, svcPeer *Peer) *Probe {
	probe := &Probe{
		ctx:        ctx,
		heartbeat:  heartbeat.NewHeartbeat(host),
		svcPeer:    svcPeer,
		runMU:      &sync.Mutex{},
		stopFunc:   func() {},
		stopFuncMU: &sync.Mutex{},
	}
	return probe
}

func (p *Probe) Run(peerID peer.ID) (statusUpdates <-chan ProbeStatus) {
	p.stopFuncMU.Lock()
	p.stopFunc()
	p.stopFuncMU.Unlock()

	p.runMU.Lock()

	var ctx context.Context

	p.stopFuncMU.Lock()
	ctx, p.stopFunc = context.WithCancel(p.ctx)
	p.stopFuncMU.Unlock()

	status := ProbeStatusOff
	ch := make(chan ProbeStatus, 1)
	ch <- status
	update := func(stat ProbeStatus) {
		if status != stat {
			status = stat
			select {
			case ch <- stat:
			case <-ch:
				ch <- stat
			}
		}
	}
	go func(ctx context.Context) {
		defer p.runMU.Unlock()
		defer func() {
			p.stopFuncMU.Lock()
			p.stopFunc()
			p.stopFuncMU.Unlock()
		}()
		defer close(ch)

		for {
			sctx, cancel := context.WithTimeout(ctx, ProbeInterval)
			switch {
			default:
				direct, err := p.heartbeat.Send(sctx, peerID)
				if err != nil {
					update(ProbeStatusOff)
					logger.Error("failed to send heartbeat to %s: %v", peerID, err)
				} else {
					p.svcPeer.Save(sctx, peerID)

					if direct {
						update(ProbeStatusDirect)
					} else {
						update(ProbeStatusRelay)
					}
				}
			}

			select {
			case <-ctx.Done():
				cancel()
				logger.Error("stop probing")
				return
			case <-sctx.Done():
				cancel()
			}
		}
	}(ctx)

	statusUpdates = ch
	return
}

func (p *Probe) Stop() (err error) {
	p.stopFuncMU.Lock()
	p.stopFunc()
	p.stopFuncMU.Unlock()
	return
}
