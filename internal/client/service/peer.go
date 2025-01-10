package service

import (
	"chat-libp2p/internal/client/service/database"
	"chat-libp2p/internal/client/service/model"
	"chat-libp2p/internal/common/protocol/identify"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.etcd.io/bbolt"
)

type Peer struct {
	ctx          context.Context
	bolt         *database.Bolt
	store        *sync.Map
	identify     *identify.Identify
	localPeerID  peer.ID
	localEVMAddr string
}

func NewPeer(ctx context.Context, bolt *database.Bolt, host host.Host) (peer *Peer) {
	evmAddr := ctx.Value(CtxKeyEVMAddr).(string)
	peer = &Peer{
		ctx:          ctx,
		bolt:         bolt,
		store:        &sync.Map{},
		identify:     identify.NewIdentify(host, evmAddr),
		localPeerID:  host.ID(),
		localEVMAddr: evmAddr,
	}
	return peer
}

func (p *Peer) Find(peerID peer.ID) (data *model.Peer, err error) {
	err = p.bolt.DB().View(func(tx *bbolt.Tx) (err error) {
		bucket, _ := BucketKeyPeer.GetBucket(tx)
		if bucket == nil {
			err = fmt.Errorf("peer id not found")
			return
		}
		key := []byte(peerID)
		buf := bucket.Get(key)
		if buf == nil {
			err = fmt.Errorf("peer id not found")
			return
		}
		data = &model.Peer{}
		if err = data.Decode(buf); err != nil {
			return
		}
		return
	})
	return
}

func (p *Peer) Save(ctx context.Context, peerID peer.ID) (err error) {
	m := &sync.Mutex{}
	m.Lock()
	if mu, loaded := p.store.LoadOrStore(peerID, m); loaded {
		m.Unlock()
		switch val := mu.(type) {
		case *sync.Mutex:
			m = val
			m.Lock()
			v, _ := p.store.Load(peerID) // double check
			if _, ok := v.(*sync.Mutex); !ok {
				m.Unlock()
				return
			}
		default:
			return
		}
	}
	defer func() {
		if err == nil {
			p.store.Store(peerID, struct{}{})
		}
		m.Unlock()
	}()
	data := &model.Peer{
		ID: peerID.String(),
	}
	err = p.bolt.DB().Update(func(tx *bbolt.Tx) (err error) {
		bucket, err := BucketKeyPeer.GetOrNewBucket(tx)
		if err != nil {
			return
		}
		key := []byte(peerID)
		if val := bucket.Get(key); val != nil {
			data.Decode(val)
			return
		}
		if peerID == p.localPeerID {
			data.EVMAddr = p.localEVMAddr
		}
		dataBytes, err := data.Encode()
		if err != nil {
			return
		}
		err = bucket.Put(key, dataBytes)
		return
	})
	if data.EVMAddr == "" {
		ctx, cancel := context.WithTimeout(ctx, time.Second*3)
		defer cancel()
		data.EVMAddr, err = p.identify.QueryEVM(ctx, peerID)
		if err != nil {
			err = fmt.Errorf("failed to query evm address: %v", err)
			return
		}
		if err = p.SetEVMAddr(peerID, data.EVMAddr); err != nil {
			err = fmt.Errorf("failed to set evm address: %v", err)
			return
		}
	}
	return
}

func (p *Peer) FindAllWithoutLocal() (data []*model.Peer, err error) {
	err = p.bolt.DB().View(func(tx *bbolt.Tx) (err error) {
		bucket, _ := BucketKeyPeer.GetBucket(tx)
		if bucket == nil {
			return
		}
		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			if peer.ID(k) == p.localPeerID {
				continue
			}
			peer := &model.Peer{}
			if err = peer.Decode(v); err != nil {
				return
			}
			data = append(data, peer)
		}
		return
	})
	return
}

func (p *Peer) SetEVMAddr(peerID peer.ID, evmAddr string) (err error) {
	err = p.bolt.DB().Update(func(tx *bbolt.Tx) (err error) {
		bucket, err := BucketKeyPeer.GetOrNewBucket(tx)
		if err != nil {
			return
		}
		data := &model.Peer{}
		key := []byte(peerID)
		if val := bucket.Get(key); val == nil {
			err = fmt.Errorf("peer does not exist")
			return
		} else if err = data.Decode(val); err != nil {
			return
		}
		data.EVMAddr = evmAddr
		dataBytes, err := data.Encode()
		if err != nil {
			return
		}
		err = bucket.Put(key, dataBytes)
		return
	})
	return
}

func (p *Peer) SetRemark(peerID peer.ID, remark string) (err error) {
	err = p.bolt.DB().Update(func(tx *bbolt.Tx) (err error) {
		bucket, err := BucketKeyPeer.GetOrNewBucket(tx)
		if err != nil {
			return
		}
		data := &model.Peer{}
		key := []byte(peerID)
		if val := bucket.Get(key); val == nil {
			err = fmt.Errorf("peer does not exist")
			return
		} else if err = data.Decode(val); err != nil {
			return
		}
		data.Remark = remark
		dataBytes, err := data.Encode()
		if err != nil {
			return
		}
		err = bucket.Put(key, dataBytes)
		return
	})
	return
}

func (p *Peer) SetPin(peerID peer.ID, pin bool) (err error) {
	err = p.bolt.DB().Update(func(tx *bbolt.Tx) (err error) {
		bucket, err := BucketKeyPeer.GetOrNewBucket(tx)
		if err != nil {
			return
		}
		data := &model.Peer{}
		key := []byte(peerID)
		if val := bucket.Get(key); val == nil {
			err = fmt.Errorf("peer does not exist")
			return
		} else if err = data.Decode(val); err != nil {
			return
		}
		data.Pin = pin
		dataBytes, err := data.Encode()
		if err != nil {
			return
		}
		err = bucket.Put(key, dataBytes)
		return
	})
	return
}
