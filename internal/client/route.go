package client

import (
	"context"
	"fmt"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/peer"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
)

type Route struct {
	*drouting.RoutingDiscovery
}

func NewRoute(dht *dht.IpfsDHT) *Route {
	route := &Route{drouting.NewRoutingDiscovery(dht)}
	return route
}

func (r *Route) FindPeer(ctx context.Context, peerID peer.ID) (addrInfo peer.AddrInfo, err error) {
	peers, err := r.FindPeers(ctx, peerID.String())
	if err != nil {
		err = fmt.Errorf("failed to find peers: %v", err)
		return
	}
	for {
		select {
		case peer, ok := <-peers:
			if !ok {
				err = fmt.Errorf("peers queue closed")
				return
			}
			if peer.ID == peerID {
				addrInfo = peer
				return
			}
		case <-ctx.Done():
			err = fmt.Errorf("find timeout")
			return
		}
	}
}
