package server

import (
	"bytes"
	"chat-libp2p/pkg/logger"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"strings"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	rhost "github.com/libp2p/go-libp2p/p2p/host/routed"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/multiformats/go-multiaddr"
)

type Server struct {
	conf *Config
	dht  *dht.IpfsDHT
	host host.Host
	halt chan struct{}
}

func NewServer(conf *Config) *Server {
	server := &Server{
		conf: conf,
		dht:  nil,
		host: nil,
		halt: make(chan struct{}),
	}
	return server
}

func (s *Server) Run(ctx context.Context) (err error) {
	var opts []libp2p.Option

	var privKey crypto.PrivKey
	if privateKey := s.conf.PrivateKey(); len(privateKey) > 0 {
		privKeyBuf, e := hex.DecodeString(privateKey)
		if e != nil {
			err = fmt.Errorf("failed to decode private key: %v", e)
			return
		}
		privk := secp256k1.PrivKeyFromBytes(privKeyBuf)
		privKey = (*crypto.Secp256k1PrivateKey)(privk)
	} else {
		privKey, _, err = crypto.GenerateKeyPair(crypto.Secp256k1, 0)
		if err != nil {
			err = fmt.Errorf("failed to generate key pair: %v", err)
			return
		}
	}
	opts = append(opts, libp2p.Identity(privKey))

	for _, addr := range s.conf.Listen() {
		opts = append(opts, libp2p.ListenAddrStrings(addr))
	}

	if publicIP := s.conf.PublicIP(); len(publicIP) != 0 {
		opts = append(opts, libp2p.AddrsFactory(func(m []multiaddr.Multiaddr) []multiaddr.Multiaddr {
			annoAddrs := make([]multiaddr.Multiaddr, 0)
			uniqAddrs := make(map[string]struct{})
			for _, maddr := range m {
				ipStr, err := maddr.ValueForProtocol(multiaddr.P_IP4)
				if err != nil {
					continue
				}
				uniqAddrs[strings.ReplaceAll(maddr.String(), ipStr, publicIP)] = struct{}{}
			}
			for addrStr := range uniqAddrs {
				maddr, err := multiaddr.NewMultiaddr(addrStr)
				if err != nil {
					continue
				}
				annoAddrs = append(annoAddrs, maddr)
			}
			return annoAddrs
		}))
		opts = append(opts, libp2p.ForceReachabilityPublic())
	}

	opts = append(opts, libp2p.EnableNATService())
	opts = append(opts, libp2p.EnableRelayService(relayv2.WithInfiniteLimits()))

	// resource
	mgr, err := rcmgr.NewResourceManager(
		rcmgr.NewFixedLimiter(rcmgr.InfiniteLimits),
		rcmgr.WithNetworkPrefixLimit([]rcmgr.NetworkPrefixLimit{{
			Network:   netip.MustParsePrefix("0.0.0.0/0"),
			ConnCount: math.MaxInt, // Unlimited
		}}, nil),
	)
	if err != nil {
		err = fmt.Errorf("failed to new resource manager: %v", err)
		return
	}
	opts = append(opts, libp2p.ResourceManager(mgr))

	s.host, err = libp2p.New(opts...)
	if err != nil {
		err = fmt.Errorf("failed to new libp2p node: %v", err)
		return
	}
	defer s.host.Close()

	s.dht, err = dht.New(ctx, s.host, dht.Mode(dht.ModeServer))
	if err != nil {
		err = fmt.Errorf("failed to init DHT: %v", err)
		return
	}
	defer s.dht.Close()

	if err = s.dht.Bootstrap(ctx); err != nil {
		err = fmt.Errorf("failed to bootstrap dht: %v", err)
		return
	}

	s.host = rhost.Wrap(s.host, s.dht)

	connectBootstrap := func(conf *Config) error {
		for _, addr := range conf.Bootstrap() {
			addrInfo, err := peer.AddrInfoFromString(addr)
			if err != nil {
				return fmt.Errorf("failed to parse peer info: %v", err)
			}
			if err := s.host.Connect(ctx, *addrInfo); err != nil {
				return fmt.Errorf("failed to connect to peer %s: %v", addrInfo, err)
			}
		}
		return nil
	}
	connectBootstrap(s.conf)
	s.conf.OnChange(connectBootstrap)

	<-s.halt
	err = errors.New("shutdown")
	return
}

func (s *Server) Close() (err error) {
	close(s.halt)
	return
}

func (s *Server) Dump() {
	buf := &bytes.Buffer{}
	buf.WriteString(fmt.Sprintf("\nnode id: %s\n", s.host.ID()))
	buf.WriteString("announce addresses:\n")
	for _, addr := range s.host.Addrs() {
		buf.WriteString(fmt.Sprintf("\t%s/p2p/%s\n", addr, s.host.ID()))
	}
	buf.WriteString("linked peers:\n")
	for _, peerId := range s.host.Network().Peers() {
		peerInfo := s.host.Peerstore().PeerInfo(peerId)
		buf.WriteString(fmt.Sprintf("\t%s\n", peerInfo.ID))
		for _, addr := range peerInfo.Addrs {
			buf.WriteString(fmt.Sprintf("\t\t%s/p2p/%s\n", addr, peerInfo.ID))
		}
	}
	buf.WriteString("dht route:\n")
	buf.WriteString(fmt.Sprintf("\t%v", s.dht.RoutingTable().ListPeers()))
	logger.Info(buf.String())
}
