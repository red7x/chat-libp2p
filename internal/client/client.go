package client

import (
	"bytes"
	"chat-libp2p/internal/client/service"
	"chat-libp2p/internal/client/service/database"
	"chat-libp2p/internal/client/service/model"
	"chat-libp2p/pkg/logger"
	"context"
	"fmt"
	"math"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	rhost "github.com/libp2p/go-libp2p/p2p/host/routed"
)

type Client struct {
	OK chan struct{}

	conf *Config
	host host.Host
	halt chan struct{}

	// service
	svcPeer    *service.Peer
	svcProbe   *service.Probe
	svcMessage *service.Message
}

func NewClient() *Client {
	client := &Client{
		OK: make(chan struct{}),

		conf: NewConfig(),
		host: nil,
		halt: make(chan struct{}),

		// service
		svcPeer:    nil,
		svcProbe:   nil,
		svcMessage: nil,
	}
	return client
}

func (c *Client) LoadConfig(buf []byte) (err error) {
	return c.conf.Load(buf)
}

func (c *Client) Run(ctx context.Context) (err error) {

	// config
	privKey, err := c.conf.GetPrivateKey()
	if err != nil {
		err = fmt.Errorf("failed to get private key: %v", err)
		return
	}

	localEVM, err := c.conf.GetEthereumAddress()
	if err != nil {
		err = fmt.Errorf("failed to get local evm address: %v", err)
		return
	}
	ctx = context.WithValue(ctx, service.CtxKeyEVMAddr, localEVM)

	bootstrap, err := c.conf.GetBootstrap()
	if err != nil {
		err = fmt.Errorf("failed to get bootstrap: %v", err)
		return
	}

	imageDir, err := c.conf.GetImageDir()
	if err != nil {
		err = fmt.Errorf("failed to get images directory: %v", err)
		return
	}
	ctx = context.WithValue(ctx, service.CtxKeyImageDir, imageDir)

	databaseDir, err := c.conf.GetDatabaseDir()
	if err != nil {
		err = fmt.Errorf("failed to get database directory: %v", err)
		return
	}

	// database
	bolt, err := database.NewBolt(filepath.Join(databaseDir, "data.db"))
	if err != nil {
		err = fmt.Errorf("failed to create bolt db: %v", err)
		return
	}
	defer bolt.Close()

	// libp2p
	resManager, err := rcmgr.NewResourceManager(
		rcmgr.NewFixedLimiter(rcmgr.InfiniteLimits),
		rcmgr.WithNetworkPrefixLimit([]rcmgr.NetworkPrefixLimit{{
			Network:   netip.MustParsePrefix("0.0.0.0/0"),
			ConnCount: math.MaxInt, // unlimited
		}}, nil),
	)
	if err != nil {
		err = fmt.Errorf("failed to new resource manager: %v", err)
		return
	}

	host, err := libp2p.New(
		libp2p.Identity(privKey),
		libp2p.ResourceManager(resManager),
		libp2p.ForceReachabilityPrivate(),
		libp2p.EnableAutoNATv2(),
		libp2p.EnableRelay(),
		libp2p.EnableAutoRelayWithStaticRelays(bootstrap),
		libp2p.NATPortMap(),
		libp2p.EnableHolePunching(),
		libp2p.ListenAddrStrings(
			"/ip4/0.0.0.0/tcp/0",
			"/ip4/0.0.0.0/udp/0/quic-v1",
		),
	)
	if err != nil {
		logger.Error("failed to new libp2p node: %v", err)
		return
	}
	defer host.Close()

	dht, err := dht.New(ctx, host, dht.Mode(dht.ModeServer))
	if err != nil {
		logger.Error("failed to init DHT: %v", err)
		return
	}
	defer dht.Close()

	if err = dht.Bootstrap(ctx); err != nil {
		logger.Error("failed to bootstrap dht: %v", err)
		return
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	route := NewRoute(dht)
	dutil.Advertise(ctx, route, host.ID().String())

	c.host = rhost.Wrap(host, route)
	defer c.host.Close()

	// services
	c.svcPeer = service.NewPeer(ctx, bolt, host)
	c.svcProbe = service.NewProbe(ctx, host, c.svcPeer)
	c.svcMessage = service.NewMessage(ctx, bolt, host, c.svcPeer)

	if err = c.svcPeer.Save(ctx, c.host.ID()); err != nil {
		logger.Error("failed to save self peer info: %v", err)
		return
	}

	close(c.OK)

	ticker := time.NewTicker(time.Second * 2)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if len(c.host.Network().Peers()) == 0 {
				err = fmt.Errorf("failed to connect to network")
				return
			}
		case <-ctx.Done():
			err = ctx.Err()
			return
		case <-c.halt:
			err = fmt.Errorf("client is closed")
			return
		}
	}
}

func (c *Client) PeerID() (peerID peer.ID) {
	<-c.OK
	return c.host.ID()
}

func (c *Client) PeerInfo(peerID peer.ID) (peer *model.Peer, err error) {
	<-c.OK
	return c.svcPeer.Find(peerID)
}

func (c *Client) PeerList() (peers []*model.Peer, err error) {
	<-c.OK
	peers, err = c.svcPeer.FindAllWithoutLocal()
	if err != nil {
		return
	}
	for _, p := range peers {
		peerID, err := peer.Decode(p.ID)
		if err != nil {
			continue
		}
		msgs, err := c.svcMessage.GetLastMessages(peerID, 0, 1)
		if err != nil || len(msgs) == 0 {
			continue
		}
		p.LastMessage = msgs[0]
	}
	sort.Sort(model.Peers(peers))
	return
}

func (c *Client) SetRemark(peerID peer.ID, remark string) (err error) {
	<-c.OK
	return c.svcPeer.SetRemark(peerID, remark)
}

func (c *Client) PinPeer(peerID peer.ID, pin bool) (err error) {
	<-c.OK
	return c.svcPeer.SetPin(peerID, pin)
}

func (c *Client) ProbeStatus(peerID peer.ID) (statusUpdates <-chan service.ProbeStatus) {
	<-c.OK
	c.svcMessage.AsyncSend(peerID)
	return c.svcProbe.Run(peerID)
}

func (c *Client) StopProbing() (err error) {
	<-c.OK
	return c.svcProbe.Stop()
}

func (c *Client) SendText(peerID peer.ID, content string, ttl time.Duration) (mid uint64, err error) {
	<-c.OK
	return c.svcMessage.Send(peerID, service.MessageTypeText, []byte(content), ttl)
}

func (c *Client) SendImage(peerID peer.ID, payload []byte) (mid uint64, err error) {
	<-c.OK
	var (
		filedir string
	)
	// payload might be a file path
	if len(payload) > 32 {
		filedir = filepath.Dir(string(payload[:32]))
	} else {
		filedir = filepath.Dir(string(payload))
	}
	if _, err = os.Stat(filedir); err == nil {
		filename := string(payload)
		payload, err = os.ReadFile(filename)
		if err != nil {
			return
		}
	}
	return c.svcMessage.Send(peerID, service.MessageTypeImage, payload, 0)
}

func (c *Client) MessageArrived() (peerID peer.ID, mid uint64, err error) {
	<-c.OK
	return c.svcMessage.Arrived()
}

func (c *Client) ReceiveMessage() (peerID peer.ID, mid uint64, err error) {
	<-c.OK
	return c.svcMessage.Receive()
}

func (c *Client) FindMessage(peerID peer.ID, mid uint64) (data *model.Message, err error) {
	<-c.OK
	return c.svcMessage.GetMessage(peerID, mid)
}

func (c *Client) FindLastMessages(peerID peer.ID, offset int, limit int) (data []*model.Message, err error) {
	<-c.OK
	return c.svcMessage.GetLastMessages(peerID, offset, limit)
}

func (c *Client) DeleteMessages(peerID peer.ID, mids []uint64) (err error) {
	<-c.OK
	return c.svcMessage.DelMessages(peerID, mids)
}

func (c *Client) FlushMessages(peerID peer.ID) (err error) {
	<-c.OK
	return c.svcMessage.FlushMessages(peerID)
}

func (c *Client) Close() (err error) {
	close(c.halt)
	return
}

func (c *Client) Dump() {
	buf := &bytes.Buffer{}
	buf.WriteString(fmt.Sprintf("\nnode id: %s\n", c.host.ID()))
	buf.WriteString("announce addresses:\n")
	for _, addr := range c.host.Addrs() {
		buf.WriteString(fmt.Sprintf("\t%s/p2p/%s\n", addr, c.host.ID()))
	}
	buf.WriteString("linked peers:\n")
	for _, peerId := range c.host.Network().Peers() {
		peerInfo := c.host.Peerstore().PeerInfo(peerId)
		buf.WriteString(fmt.Sprintf("\t%s\n", peerInfo.ID))
		for _, addr := range peerInfo.Addrs {
			buf.WriteString(fmt.Sprintf("\t\t%s/p2p/%s\n", addr, peerInfo.ID))
		}
	}
	logger.Info(buf.String())
}
