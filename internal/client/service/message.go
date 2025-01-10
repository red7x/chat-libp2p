package service

import (
	"chat-libp2p/internal/client/service/database"
	"chat-libp2p/internal/client/service/model"
	proto "chat-libp2p/internal/common/protocol"
	"chat-libp2p/internal/common/protocol/image"
	"chat-libp2p/internal/common/protocol/text"
	"chat-libp2p/pkg/logger"
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.etcd.io/bbolt"
)

var (
	MessageSendTimeout   time.Duration = time.Second * 5
	MessageImageTimeout  time.Duration = time.Minute
	MessageSendingPeriod time.Duration = time.Second * 10
	MessageNotifierSize  int           = 1000
)

const (
	MessageTypeText  string = "text/plain"
	MessageTypeImage string = "image/any"
)

type peerValue struct {
	peerID peer.ID
	value  any
}

type Message struct {
	ctx             context.Context
	bolt            *database.Bolt
	text            *text.Text
	image           *image.Image
	imageDir        string
	sending         *sync.Map
	svcPeer         *Peer
	destoryUpdated  chan struct{}
	receiveNotifier chan peerValue
	arrivedNotifier chan peerValue
}

func NewMessage(ctx context.Context, bolt *database.Bolt, host host.Host, svcPeer *Peer) (message *Message) {
	message = &Message{
		ctx:             ctx,
		bolt:            bolt,
		text:            text.NewText(host),
		image:           image.NewImage(host),
		imageDir:        ctx.Value(CtxKeyImageDir).(string),
		sending:         &sync.Map{},
		svcPeer:         svcPeer,
		destoryUpdated:  make(chan struct{}, 1),
		receiveNotifier: make(chan peerValue, MessageNotifierSize),
		arrivedNotifier: make(chan peerValue, MessageNotifierSize),
	}
	go message.destory()
	go message.receive()
	return message
}

func (m *Message) destory() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		ticker.Reset(time.Second * 10)
		err := m.bolt.DB().Update(func(tx *bbolt.Tx) (err error) {
			destoryBucket, _ := BucketKeyDestroyMsg.GetBucket(tx)
			if destoryBucket == nil {
				return
			}
			c := destoryBucket.Cursor()
			for k, v := c.First(); k != nil; k, v = c.Next() {
				timestamp := binary.BigEndian.Uint64(k)
				if wait := time.Since(time.Unix(0, int64(timestamp))); wait > 0 {
					ticker.Reset(wait)
					break
				}
				var (
					peerKey    = v[8:]
					messageKey = v[:8]
				)
				dialogueBucket, _ := BucketKeyDialogue.Suffix(peerKey).GetBucket(tx)
				if dialogueBucket != nil {
					if err = dialogueBucket.Delete(messageKey); err != nil {
						return
					}
				}
				pendingBucket, _ := BucketKeyMsgPending.Suffix(peerKey).GetBucket(tx)
				if pendingBucket != nil {
					if err = pendingBucket.Delete(messageKey); err != nil {
						return
					}
				}
			}
			return
		})
		if err != nil {
			logger.Error("failed to destory message: %v", err)
		}
		select {
		case <-ticker.C:
		case <-m.destoryUpdated:
		case <-m.ctx.Done():
			logger.Error("destory loop stop: %v", m.ctx.Err())
			return
		}
	}
}

func (m *Message) receive() {
	var (
		peerID        peer.ID
		peerRequest   proto.PeerRequest
		textReceiver  = m.text.Receiver(m.ctx)
		imageReceiver = m.image.Receiver(m.ctx)
	)
	for {
		select {
		case peerRequest = <-textReceiver:
		case peerRequest = <-imageReceiver:
		case <-m.ctx.Done():
			logger.Error("receive loop stop: %v", m.ctx.Err())
			return
		}
		peerID = peerRequest.ID
		m.AsyncSend(peerID)
		switch req := peerRequest.Request.(type) {
		case *text.ReqSyncText:
			m.svcPeer.Save(m.ctx, peerID)
			m.syncText(peerID, req)
		case *image.ReqSyncHash:
			m.svcPeer.Save(m.ctx, peerID)
			m.syncHash(peerID, req)
		case *image.ReqSyncBytes:
			m.syncBytes(peerID, req)
		case *text.ReqExistence:
			m.textExist(peerID, req)
		case *image.ReqExistence:
			m.imageExist(peerID, req)
		}
	}
}

func (m *Message) syncText(peerID peer.ID, req *text.ReqSyncText) {
	var (
		id  uint64
		ttl = req.TTL()
	)
	err := m.bolt.DB().Update(func(tx *bbolt.Tx) (err error) {
		peerKey := []byte(peerID)
		dialogueBucket, err := BucketKeyDialogue.Suffix(peerKey).GetOrNewBucket(tx)
		if err != nil {
			return
		}
		id, err = dialogueBucket.NextSequence()
		if err != nil {
			return
		}
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, id)
		data := &model.Message{
			ID:        id,
			Type:      MessageTypeText,
			Payload:   req.ContentBytes(),
			Reached:   true,
			Initiator: false,
			CreatedAt: time.Now(),
			TTL:       ttl,
		}
		dataBytes, err := data.Encode()
		if err != nil {
			return
		} else if err = dialogueBucket.Put(key, dataBytes); err != nil {
			return
		}
		receivedBucket, err := BucketKeyMsgReceived.Suffix(peerKey).GetOrNewBucket(tx)
		if err != nil {
			return
		}
		binary.BigEndian.PutUint64(key, req.ID())
		if err = receivedBucket.Put(key, nil); err != nil {
			return
		}
		if ttl > 0 {
			destoryBucket, e := BucketKeyDestroyMsg.GetOrNewBucket(tx)
			if e != nil {
				err = fmt.Errorf("failed to get destory bucket: %v", e)
				return
			}
			timestamp := uint64(time.Now().Add(ttl).UnixNano())
			binary.BigEndian.PutUint64(key, timestamp)
			value := make([]byte, 8)
			binary.BigEndian.PutUint64(value, id)
			value = append(value, peerKey...)
			if err = destoryBucket.Put(key, value); err != nil {
				return
			}
		}
		return
	})
	if err != nil {
		logger.Error("failed to save received message: %v", err)
		return
	} else if ttl > 0 {
		select {
		case m.destoryUpdated <- struct{}{}:
		default:
		}
	}
	resp := text.NewRespSyncText(req.ID(), true)
	go func() {
		if err := m.text.Send(m.ctx, peerID, resp); err != nil {
			logger.Error("failed to send response of save text: %v", err)
		}
	}()
	select {
	case m.receiveNotifier <- peerValue{peerID, id}:
	default:
	}
}

func (m *Message) getSubdir(filebase string) (subdir string, err error) {
	m.bolt.DB().View(func(tx *bbolt.Tx) (err error) {
		imagesBucket, _ := BucketKeyImages.GetBucket(tx)
		if imagesBucket == nil {
			return
		}
		val := imagesBucket.Get([]byte(filebase))
		if val != nil {
			return
		}
		subdir = string(val)
		return
	})
	if subdir == "" {
		subdir = time.Now().Format(fmt.Sprintf("2006%c01", os.PathSeparator))
		err = m.bolt.DB().Update(func(tx *bbolt.Tx) (err error) {
			imagesBucket, err := BucketKeyImages.GetOrNewBucket(tx)
			if err != nil {
				return
			}
			err = imagesBucket.Put([]byte(filebase), []byte(subdir))
			return
		})
		if err != nil {
			err = fmt.Errorf("failed to save subdirectory: %v", err)
			return
		}
	}
	imageDir := filepath.Join(m.imageDir, subdir)
	if err = os.MkdirAll(imageDir, os.ModePerm); err != nil {
		err = fmt.Errorf("failed to create image directory %s: %v", imageDir, err)
		return
	}
	return
}

func (m *Message) syncHash(peerID peer.ID, req *image.ReqSyncHash) {
	subdir, err := m.getSubdir(req.Filebase())
	if err != nil {
		logger.Error("failed to assign image directory: %v", err)
		return
	}
	filename := filepath.Join(m.imageDir, subdir, req.Filebase())
	switch {
	default:
		data, err := os.ReadFile(filename)
		if err != nil {
			break
		}
		hash := md5.Sum(data)
		if hex.EncodeToString(hash[:]) != req.Hash() {
			os.Remove(filename)
			break
		}
		// success
		var (
			id uint64
		)
		err = m.bolt.DB().Update(func(tx *bbolt.Tx) (err error) {
			peerKey := []byte(peerID)
			dialogueBucket, err := BucketKeyDialogue.Suffix(peerKey).GetOrNewBucket(tx)
			if err != nil {
				return
			}
			id, err = dialogueBucket.NextSequence()
			if err != nil {
				return
			}
			key := make([]byte, 8)
			binary.BigEndian.PutUint64(key, id)
			data := &model.Message{
				ID:        id,
				Type:      MessageTypeImage,
				Payload:   []byte(filename),
				Reached:   true,
				Initiator: false,
				CreatedAt: time.Now(),
				TTL:       0, // image not support ttl
			}
			dataBytes, err := data.Encode()
			if err != nil {
				return
			} else if err = dialogueBucket.Put(key, dataBytes); err != nil {
				return
			}
			receivedBucket, err := BucketKeyMsgReceived.Suffix(peerKey).GetOrNewBucket(tx)
			if err != nil {
				return
			}
			binary.BigEndian.PutUint64(key, req.ID())
			if err = receivedBucket.Put(key, nil); err != nil {
				return
			}
			return
		})
		if err != nil {
			logger.Error("failed to save received message: %v", err)
			return
		}
		resp := image.NewRespSyncHash(req.ID(), true, "") // when success, subdir is not necessary
		go func() {
			if err := m.image.Send(m.ctx, peerID, resp); err != nil {
				logger.Error("failed to send response of sync hash: %v", err)
			}
		}()
		select {
		case m.receiveNotifier <- peerValue{peerID, id}:
		default:
		}
		return
	}
	// failure
	resp := image.NewRespSyncHash(req.ID(), false, subdir)
	go func() {
		if err := m.image.Send(m.ctx, peerID, resp); err != nil {
			logger.Error("failed to send response of sync hash: %v", err)
		}
	}()
}

func (m *Message) syncBytes(peerID peer.ID, req *image.ReqSyncBytes) {
	var (
		id      uint64
		payload = req.Payload()
	)
	contentType := http.DetectContentType(payload)
	if !strings.HasPrefix(contentType, "image/") {
		logger.Error("illegal image format")
		return
	}
	exts, err := mime.ExtensionsByType(contentType)
	if len(exts) == 0 || err != nil {
		logger.Error("failed to get extension: %v", err)
		return
	}
	ext := exts[0]
	for _, val := range exts {
		if len(val) > 1 && val[1:] == contentType[6:] {
			ext = val
			break
		}
	}
	hash := md5.Sum(payload)
	hashStr := hex.EncodeToString(hash[:])
	imageDir := filepath.Join(m.imageDir, req.Subdir())
	if err := os.MkdirAll(imageDir, os.ModePerm); err != nil {
		logger.Error("failed to create image directory %s: %v", imageDir, err)
		return
	}
	filename := filepath.Join(imageDir, hashStr+ext)
	if err := os.WriteFile(filename, payload, os.ModePerm); err != nil {
		logger.Error("failed to write image file: %v", err)
		return
	}
	// success
	err = m.bolt.DB().Update(func(tx *bbolt.Tx) (err error) {
		peerKey := []byte(peerID)
		dialogueBucket, err := BucketKeyDialogue.Suffix(peerKey).GetOrNewBucket(tx)
		if err != nil {
			return
		}
		id, err = dialogueBucket.NextSequence()
		if err != nil {
			return
		}
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, id)
		data := &model.Message{
			ID:        id,
			Type:      MessageTypeImage,
			Payload:   []byte(filename),
			Reached:   true,
			Initiator: false,
			CreatedAt: time.Now(),
			TTL:       0, // image not support ttl
		}
		dataBytes, err := data.Encode()
		if err != nil {
			return
		} else if err = dialogueBucket.Put(key, dataBytes); err != nil {
			return
		}
		receivedBucket, err := BucketKeyMsgReceived.Suffix(peerKey).GetOrNewBucket(tx)
		if err != nil {
			return
		}
		binary.BigEndian.PutUint64(key, req.ID())
		if err = receivedBucket.Put(key, nil); err != nil {
			return
		}
		return
	})
	if err != nil {
		logger.Error("failed to save received message: %v", err)
		return
	}
	resp := image.NewRespSyncBytes(req.ID(), true)
	go func() {
		if err := m.image.Send(m.ctx, peerID, resp); err != nil {
			logger.Error("failed to send response of sync hash: %v", err)
		}
	}()
	select {
	case m.receiveNotifier <- peerValue{peerID, id}:
	default:
	}
}

func (m *Message) textExist(peerID peer.ID, req *text.ReqExistence) {
	exist := false
	m.bolt.DB().View(func(tx *bbolt.Tx) (err error) {
		peerKey := []byte(peerID)
		receivedBucket, _ := BucketKeyMsgReceived.Suffix(peerKey).GetBucket(tx)
		if receivedBucket == nil {
			return
		}
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, req.ID())
		if receivedBucket.Get(key) != nil {
			exist = true
		}
		return
	})
	resp := text.NewRespExistence(req.ID(), exist)
	go func() {
		if err := m.text.Send(m.ctx, peerID, resp); err != nil {
			logger.Error("failed to send response of text existence: %v", err)
		}
	}()
}

func (m *Message) imageExist(peerID peer.ID, req *image.ReqExistence) {
	exist := false
	m.bolt.DB().View(func(tx *bbolt.Tx) (err error) {
		peerKey := []byte(peerID)
		receivedBucket, _ := BucketKeyMsgReceived.Suffix(peerKey).GetBucket(tx)
		if receivedBucket == nil {
			return
		}
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, req.ID())
		if receivedBucket.Get(key) != nil {
			exist = true
		}
		return
	})
	resp := image.NewRespExistence(req.ID(), exist)
	go func() {
		if err := m.image.Send(m.ctx, peerID, resp); err != nil {
			logger.Error("failed to send response of image existence: %v", err)
		}
	}()
}

func (m *Message) Send(peerID peer.ID, mtype string, payload []byte, ttl time.Duration) (id uint64, err error) {
	switch mtype {
	case MessageTypeText:
	case MessageTypeImage:
		contentType := http.DetectContentType(payload)
		if !strings.HasPrefix(contentType, "image/") {
			err = fmt.Errorf("illegal image format")
			return
		}
		exts, e := mime.ExtensionsByType(contentType)
		if len(exts) == 0 || e != nil {
			err = fmt.Errorf("failed to get extension: %v", e)
			return
		}
		ext := exts[0]
		for _, val := range exts {
			if len(val) > 1 && val[1:] == contentType[6:] {
				ext = val
				break
			}
		}
		hash := md5.Sum(payload)
		hashStr := hex.EncodeToString(hash[:])
		filename := fmt.Sprintf("%s%s", hashStr, ext)
		subdir, e := m.getSubdir(filename)
		if e != nil {
			err = fmt.Errorf("failed to assign subdirectory: %v", e)
			return
		}
		filename = filepath.Join(m.imageDir, subdir, filename)
		if _, e = os.Stat(filename); !os.IsNotExist(e) {
			payload = []byte(filename)
			break
		}
		if err = os.WriteFile(filename, payload, os.ModePerm); err != nil {
			err = fmt.Errorf("failed to write image cache file: %v", err)
			return
		}
		payload = []byte(filename)
	default:
		err = fmt.Errorf("unsupported mtype")
		return
	}
	err = m.bolt.DB().Update(func(tx *bbolt.Tx) (err error) {
		peerKey := []byte(peerID)
		dialogueBucket, err := BucketKeyDialogue.Suffix(peerKey).GetOrNewBucket(tx)
		if err != nil {
			return
		}
		id, err = dialogueBucket.NextSequence()
		if err != nil {
			return
		}
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, id)
		data := &model.Message{
			ID:        id,
			Type:      mtype,
			Payload:   payload,
			Reached:   false,
			Initiator: true,
			CreatedAt: time.Now(),
			TTL:       ttl,
		}
		dataBytes, err := data.Encode()
		if err != nil {
			return
		} else if err = dialogueBucket.Put(key, dataBytes); err != nil {
			return
		}
		pendingBucket, err := BucketKeyMsgPending.Suffix(peerKey).GetOrNewBucket(tx)
		if err != nil {
			return
		} else if err = pendingBucket.Put(key, nil); err != nil {
			return
		}
		if ttl > 0 {
			destoryBucket, e := BucketKeyDestroyMsg.GetOrNewBucket(tx)
			if e != nil {
				err = fmt.Errorf("failed to get destory bucket: %v", e)
				return
			}
			timestamp := uint64(time.Now().Add(ttl).UnixNano())
			binary.BigEndian.PutUint64(key, timestamp)
			value := make([]byte, 8)
			binary.BigEndian.PutUint64(value, id)
			value = append(value, peerKey...)
			if err = destoryBucket.Put(key, value); err != nil {
				return
			}
		}
		return
	})
	if err != nil {
		err = fmt.Errorf("failed to save message: %v", err)
		return
	} else if ttl > 0 {
		select {
		case m.destoryUpdated <- struct{}{}:
		default:
		}
	}
	m.AsyncSend(peerID)
	return
}

func (m *Message) AsyncSend(peerID peer.ID) {
	var (
		send        = make([]*model.Message, 0)
		check       = make([]*model.Message, 0)
		pendingKey  = BucketKeyMsgPending.Suffix([]byte(peerID))
		dialogueKey = BucketKeyDialogue.Suffix([]byte(peerID))
	)
	m.bolt.DB().View(func(tx *bbolt.Tx) (err error) {
		pendingBucket, _ := pendingKey.GetBucket(tx)
		if pendingBucket == nil {
			return
		}
		dialogueBucket, _ := dialogueKey.GetBucket(tx)
		if dialogueBucket == nil {
			return
		}
		c := pendingBucket.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			if val := dialogueBucket.Get(k); val != nil {
				data := &model.Message{}
				data.Decode(val)
				if time.Since(data.CreatedAt) > MessageSendingPeriod || time.Now().Before(data.CreatedAt) {
					check = append(check, data)
					continue
				}
				send = append(send, data)
			} else {
				c.Delete()
			}
		}
		return
	})
	val, _ := m.sending.LoadOrStore(peerID, &sync.Map{})
	recorder := val.(*sync.Map)
	for _, data := range check {
		if _, ok := recorder.LoadOrStore(data.ID, struct{}{}); ok {
			continue
		}
		go func(data *model.Message) {
			defer recorder.Delete(data.ID)
			var (
				err  error
				req  proto.Request
				resp proto.Response
			)
			ctx, cancel := context.WithTimeout(m.ctx, MessageSendTimeout)
			switch data.Type {
			case MessageTypeText:
				req = text.NewReqExistence(data.ID)
				resp, err = m.text.Request(ctx, peerID, req)
			case MessageTypeImage:
				req = image.NewReqExistence(data.ID)
				resp, err = m.image.Request(ctx, peerID, req)
			}
			cancel()
			if err != nil {
				logger.Error("failed to request message existence: %v", err)
				return
			}
			err = m.bolt.DB().Update(func(tx *bbolt.Tx) (err error) {
				key := make([]byte, 8)
				binary.BigEndian.PutUint64(key, data.ID)
				if resp.OK() {
					var dialogueBucket *bbolt.Bucket
					dialogueBucket, err = dialogueKey.GetOrNewBucket(tx)
					if err != nil {
						return
					}
					if val := dialogueBucket.Get(key); val != nil {
						data := &model.Message{}
						data.Decode(val)
						data.Reached = true
						dataBytes, e := data.Encode()
						if e != nil {
							err = fmt.Errorf("failed to encode model: %v", e)
							return
						} else if err = dialogueBucket.Put(key, dataBytes); err != nil {
							return
						}
					}
				}
				pendingBucket, err := pendingKey.GetOrNewBucket(tx)
				if err != nil {
					return
				}
				err = pendingBucket.Delete(key)
				return
			})
			if err != nil {
				logger.Error("failed to update data: %v", err)
				return
			} else if data.Reached {
				select {
				case m.arrivedNotifier <- peerValue{peerID, data.ID}:
				default:
				}
			}
		}(data)
	}
	for _, data := range send {
		if _, ok := recorder.LoadOrStore(data.ID, struct{}{}); ok {
			continue
		}
		go func(data *model.Message) {
			defer recorder.Delete(data.ID)
			var (
				err  error
				req  proto.Request
				resp proto.Response
			)
			ctx, cancel := context.WithTimeout(m.ctx, MessageSendTimeout)
			switch data.Type {
			case MessageTypeText:
				req = text.NewReqSyncText(data.ID, data.TTL, string(data.Payload))
				_, err = m.text.Request(ctx, peerID, req)
			case MessageTypeImage:
				filename := string(data.Payload)
				req = image.NewReqSyncHash(data.ID, filepath.Base(filename))
				resp, err = m.image.Request(ctx, peerID, req)
				if err != nil || resp.OK() {
					break
				}
				resp := resp.(*image.RespSyncHash)
				payload, e := os.ReadFile(filename)
				if e != nil {
					err = fmt.Errorf("failed to read image file: %v", e)
					break
				}
				ctx, cancel := context.WithTimeout(m.ctx, MessageImageTimeout)
				req = image.NewReqSyncBytes(data.ID, resp.Subdir(), payload)
				_, err = m.image.Request(ctx, peerID, req)
				cancel()
			}
			cancel()
			if err != nil {
				logger.Error("failed to order send message: %v", err)
				return
			}
			err = m.bolt.DB().Update(func(tx *bbolt.Tx) (err error) {
				dialogueBucket, err := dialogueKey.GetOrNewBucket(tx)
				if err != nil {
					return
				}
				key := make([]byte, 8)
				binary.BigEndian.PutUint64(key, data.ID)
				if val := dialogueBucket.Get(key); val != nil {
					data := &model.Message{}
					data.Decode(val)
					data.Reached = true
					dataBytes, e := data.Encode()
					if e != nil {
						err = fmt.Errorf("failed to encode model: %v", e)
						return
					} else if err = dialogueBucket.Put(key, dataBytes); err != nil {
						return
					}
				}
				pendingBucket, err := pendingKey.GetOrNewBucket(tx)
				if err != nil {
					return
				}
				err = pendingBucket.Delete(key)
				return
			})
			if err == nil {
				select {
				case m.arrivedNotifier <- peerValue{peerID, data.ID}:
				default:
				}
			} else {
				logger.Error("failed to update data: %v", err)
			}
		}(data)
	}
}

func (m *Message) Receive() (peerID peer.ID, id uint64, err error) {
	select {
	case peerValue := <-m.receiveNotifier:
		peerID, id = peerValue.peerID, peerValue.value.(uint64)
	case <-m.ctx.Done():
		err = fmt.Errorf("message service closed")
	}
	return
}

func (m *Message) Arrived() (peerID peer.ID, id uint64, err error) {
	select {
	case peerValue := <-m.arrivedNotifier:
		peerID, id = peerValue.peerID, peerValue.value.(uint64)
	case <-m.ctx.Done():
		err = fmt.Errorf("message service closed")
	}
	return
}

func (m *Message) GetMessage(peerID peer.ID, id uint64) (data *model.Message, err error) {
	err = m.bolt.DB().View(func(tx *bbolt.Tx) (err error) {
		peerKey := []byte(peerID)
		dialogueBucket, err := BucketKeyDialogue.Suffix(peerKey).GetBucket(tx)
		if err != nil {
			return
		} else if dialogueBucket == nil {
			err = fmt.Errorf("cann't find any data")
			return
		}
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, id)
		val := dialogueBucket.Get(key)
		if val == nil {
			err = fmt.Errorf("cann't find any data")
			return
		}
		data = &model.Message{}
		err = data.Decode(val)
		return
	})
	return
}

func (m *Message) GetLastMessages(peerID peer.ID, offset int, limit int) (data []*model.Message, err error) {
	err = m.bolt.DB().View(func(tx *bbolt.Tx) (err error) {
		peerKey := []byte(peerID)
		dialogueBucket, err := BucketKeyDialogue.Suffix(peerKey).GetBucket(tx)
		if err != nil || dialogueBucket == nil {
			return
		}
		c := dialogueBucket.Cursor()
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			d := &model.Message{}
			if err = d.Decode(v); err != nil {
				return
			}
			data = append([]*model.Message{d}, data...)
		}
		return
	})
	return
}

func (m *Message) DelMessages(peerID peer.ID, ids []uint64) (err error) {
	err = m.bolt.DB().Update(func(tx *bbolt.Tx) (err error) {
		peerKey := []byte(peerID)
		dialogueBucket, err := BucketKeyDialogue.Suffix(peerKey).GetOrNewBucket(tx)
		if err != nil {
			return
		}
		var (
			key  = make([]byte, 8)
			errs = make([]error, 0)
		)
		for _, id := range ids {
			binary.BigEndian.PutUint64(key, id)
			if err := dialogueBucket.Delete(key); err != nil {
				errs = append(errs, err)
			}
		}
		if len(errs) > 0 {
			err = errors.Join(errs...)
		}
		return
	})
	return
}

func (m *Message) FlushMessages(peerID peer.ID) (err error) {
	err = m.bolt.DB().Update(func(tx *bbolt.Tx) (err error) {
		peerKey := []byte(peerID)
		err = BucketKeyDialogue.Suffix(peerKey).TruncateBucket(tx)
		return
	})
	return
}
