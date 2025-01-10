package mobile

import (
	"bufio"
	"bytes"
	"chat-libp2p/internal/client"
	"chat-libp2p/internal/client/service"
	"chat-libp2p/pkg/logger"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/rs/xid"
)

const (
	StatusOff byte = iota
	StatusRelay
	StatusDirect
)

const (
	EventMsgArrived  string = "message_arrived"
	EventMsgReceived string = "message_received"
)

type Event interface {
	Name() string
	ToArrived() *Arrived
	ToMessage() *Message
}

type Arrived struct {
	PeerID    string `json:"peer_id"`
	MessageID string `json:"message_id"`
}

func (a *Arrived) Name() string {
	return EventMsgArrived
}

func (a *Arrived) ToArrived() *Arrived {
	return a
}

func (a *Arrived) ToMessage() *Message {
	return nil
}

const (
	MsgTypeText  string = "text/plain"
	MsgTypeImage string = "image/any"
)

type Message struct {
	PeerID    string   `json:"peer_id"`
	ID        string   `json:"message_id"`
	Type      string   `json:"type"`
	Payload   []byte   `json:"payload"`
	Reached   bool     `json:"reached"`
	Initiator bool     `json:"initiator"`
	CreatedAt int64    `json:"created_at"`
	TTL       int64    `json:"ttl,omitempty"`
	next      *Message `json:"-"`
}

func (m *Message) Name() string {
	return EventMsgReceived
}

func (m *Message) ToArrived() *Arrived {
	return nil
}

func (m *Message) ToMessage() *Message {
	return m
}

func (m *Message) Next() *Message {
	return m.next
}

// Append append the message object
func (m *Message) Append(msg *Message) {
	m.next = msg
}

type Peer struct {
	ID          string   `json:"peer_id"`
	EVMAddr     string   `json:"evm_addr"`
	Remark      string   `json:"remark"`
	Pin         bool     `json:"pin"`
	LastMessage *Message `json:"last_message"`
	next        *Peer    `json:"-"`
}

func (p *Peer) Next() *Peer {
	return p.next
}

var (
	sdk = &SDK{
		client:     nil,
		clientMU:   &sync.Mutex{},
		notifier:   nil,
		notifierMU: &sync.Mutex{},
		session:    nil,
		sessionMU:  &sync.Mutex{},
		events:     make(chan Event, 1000),
		stopper:    make(chan context.CancelFunc, 1),
	}
	DebugMode bool
)

// TODO remove this init function
func init() {
	if !DebugMode {
		return
	}
	out, in := io.Pipe()
	prefix := fmt.Sprintf("[%s] ", xid.New().String())
	logger.RegisterLogger(log.LstdFlags|log.Lmicroseconds|log.Lmsgprefix|log.Llongfile, prefix, in)
	chuck := make([][]byte, 0)
	chuckMU := &sync.Mutex{}
	go func() {
		wait := 0
		times := 0
		ticker := time.NewTicker(time.Second * 3)
		defer ticker.Stop()
		for range ticker.C {
			chuckMU.Lock()
			content := bytes.Join(chuck, []byte{'\n'})
			chuck = chuck[:0]
			chuckMU.Unlock()
			if wait != 0 {
				wait--
			}
			if len(content) > 0 {
				content = append(content, '\n')
				if wait == 0 {
					_, err := http.Post("https://dev.cowcow.live/collect/", "text/plain", bytes.NewBuffer(content))
					if err == nil {
						times = 0
					} else {
						times = (times + 1) * 2
						wait += times
					}
				}
			}
		}
	}()
	go func() {
		reader := bufio.NewReader(out)
		for {
			line, _, err := reader.ReadLine()
			if err != nil {
				return
			}
			chuckMU.Lock()
			chuck = append(chuck, line)
			chuckMU.Unlock()
		}
	}()
}

// GetSDK must call this to get sdk singleton
func GetSDK() *SDK {
	return sdk
}

type SDK struct {
	client     *client.Client
	clientMU   *sync.Mutex
	notifier   *Notifier
	notifierMU *sync.Mutex
	session    *Session
	sessionMU  *sync.Mutex
	events     chan Event
	stopper    chan context.CancelFunc
}

func (s *SDK) getClient() *client.Client {
	s.clientMU.Lock()
	defer s.clientMU.Unlock()
	return s.client
}

// Init initial the sdk with specific config when starting app
func (s *SDK) Init(config []byte) (err error) {
	ctx, cancel := context.WithCancel(context.Background())
	wait := true
	for wait {
		select {
		case s.stopper <- cancel:
			wait = false
		case stop := <-s.stopper:
			stop()
		}
	}

	result := make(chan error, 1)

	go func(ctx context.Context) {
		defer cancel()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		cancel := context.CancelFunc(func() {})
		defer cancel()

		for {
			client := client.NewClient()
			if err := client.LoadConfig(config); err != nil {
				select {
				case result <- fmt.Errorf("failed to load config: %v", err):
				default:
				}
			}
			s.clientMU.Lock()
			s.client = client
			s.clientMU.Unlock()
			go func() {
				for {
					peerID, mid, err := client.ReceiveMessage()
					if err != nil {
						logger.Error("failed to receive message: %v", err)
						return
					}
					data, err := client.FindMessage(peerID, mid)
					if err != nil {
						logger.Error("can't find message %d from %s: %v", mid, peerID, err)
						continue
					}
					msg := &Message{
						PeerID:    peerID.String(),
						ID:        strconv.FormatUint(data.ID, 10),
						Type:      data.Type,
						Payload:   data.Payload,
						Reached:   data.Reached,
						Initiator: data.Initiator,
						CreatedAt: data.CreatedAt.UnixMilli(),
					}
					select {
					case s.events <- msg:
					default:
						logger.Warn("lost message: %v", msg)
					}
				}
			}()
			go func() {
				for {
					peerID, mid, err := client.MessageArrived()
					if err != nil {
						logger.Error("failed to get message arrived info: %v", err)
						return
					}
					arv := &Arrived{
						PeerID:    peerID.String(),
						MessageID: strconv.FormatUint(mid, 10),
					}
					select {
					case s.events <- arv:
					default:
						logger.Warn("lost arrived info: %v", arv)
					}
				}
			}()
			sctx, scancel := context.WithCancel(ctx)
			cancel()
			cancel = scancel
			go func() {
				select {
				case <-client.OK:
					select {
					case result <- nil:
					default:
					}
				case <-sctx.Done():
				}
			}()
			if err := client.Run(sctx); err != nil {
				select {
				case result <- fmt.Errorf("client exit: %v", err):
				default:
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}(ctx)

	err = <-result
	return
}

func (s *SDK) GetPeerID() (peerID string) {
	return s.getClient().PeerID().String()
}

// GetNotifier notifier notify events such as message arrived and message received
func (s *SDK) GetNotifier() (notifier *Notifier) {
	s.notifierMU.Lock()
	defer s.notifierMU.Unlock()
	if s.notifier != nil {
		s.notifier.Close()
	}
	s.notifier = &Notifier{
		sdk:      s,
		halt:     make(chan struct{}),
		haltOnce: &sync.Once{},
	}
	return s.notifier
}

// OpenSession call this when open a dialogue with the target user peer
func (s *SDK) OpenSession(peerID string) (session *Session, err error) {
	tPeerID, err := peer.Decode(peerID)
	if err != nil {
		err = fmt.Errorf("illegal peer id: %v", err)
		return
	}
	s.sessionMU.Lock()
	defer s.sessionMU.Unlock()
	if s.session != nil {
		s.session.Close()
	}
	session = &Session{
		sdk:           s,
		peerID:        tPeerID,
		statusUpdates: s.getClient().ProbeStatus(tPeerID),
		halt:          make(chan struct{}),
		haltOnce:      &sync.Once{},
	}
	s.session = session
	return
}

// PinSession pin the session to the top of the list
func (s *SDK) PinSession(peerID string) (err error) {
	tPeerID, err := peer.Decode(peerID)
	if err != nil {
		err = fmt.Errorf("illegal peer id: %v", err)
		return
	}
	return s.getClient().PinPeer(tPeerID, true)
}

// UnpinSession unpin the session
func (s *SDK) UnpinSession(peerID string) (err error) {
	tPeerID, err := peer.Decode(peerID)
	if err != nil {
		err = fmt.Errorf("illegal peer id: %v", err)
		return
	}
	return s.getClient().PinPeer(tPeerID, false)
}

// SetRemark set the remark of the target user peer
func (s *SDK) SetRemark(peerID string, remark string) (err error) {
	pid, err := peer.Decode(peerID)
	if err != nil {
		err = fmt.Errorf("illegal peer id: %v", err)
		return
	}
	return s.getClient().SetRemark(pid, remark)
}

// GetPeerInfo get the target user peer information
func (s *SDK) GetPeerInfo(peerID string) (peerInfo *Peer, err error) {
	pid, err := peer.Decode(peerID)
	if err != nil {
		err = fmt.Errorf("illegal peer id: %v", err)
		return
	}
	info, err := s.getClient().PeerInfo(pid)
	if err != nil {
		return
	}
	peerInfo = &Peer{
		ID:      peerID,
		EVMAddr: info.EVMAddr,
		Remark:  info.Remark,
		Pin:     info.Pin,
	}
	return
}

// GetPeerList get all peers information
func (s *SDK) GetPeerList() (peers *Peer, err error) {
	data, err := s.getClient().PeerList()
	if err != nil {
		return
	}
	var peer *Peer
	for _, val := range data {
		if peers == nil {
			peer = &Peer{
				ID:      val.ID,
				EVMAddr: val.EVMAddr,
				Remark:  val.Remark,
				Pin:     val.Pin,
			}
			if val.LastMessage != nil {
				peer.LastMessage = &Message{
					PeerID:    val.ID,
					ID:        strconv.FormatUint(val.LastMessage.ID, 10),
					Type:      val.LastMessage.Type,
					Payload:   val.LastMessage.Payload,
					Reached:   val.LastMessage.Reached,
					Initiator: val.LastMessage.Initiator,
					CreatedAt: val.LastMessage.CreatedAt.UnixMilli(),
					TTL:       val.LastMessage.TTL.Milliseconds(),
				}
			}
			peers = peer
		} else {
			peer.next = &Peer{
				ID:      val.ID,
				EVMAddr: val.EVMAddr,
				Remark:  val.Remark,
				Pin:     val.Pin,
			}
			if val.LastMessage != nil {
				peer.next.LastMessage = &Message{
					PeerID:    val.ID,
					ID:        strconv.FormatUint(val.LastMessage.ID, 10),
					Type:      val.LastMessage.Type,
					Payload:   val.LastMessage.Payload,
					Reached:   val.LastMessage.Reached,
					Initiator: val.LastMessage.Initiator,
					CreatedAt: val.LastMessage.CreatedAt.UnixMilli(),
					TTL:       val.LastMessage.TTL.Milliseconds(),
				}
			}
			peer = peer.next
		}
	}
	return
}

// DeleteDialogue delete the target user all dialogue message
func (s *SDK) DeleteDialogue(peerID string) (err error) {
	pid, err := peer.Decode(peerID)
	if err != nil {
		err = fmt.Errorf("illegal peer id: %v", err)
		return
	}
	return s.getClient().FlushMessages(pid)
}

func (s *SDK) Close() (err error) {
	s.notifierMU.Lock()
	if s.notifier != nil {
		s.notifier.Close()
	}
	s.notifier = nil
	s.notifierMU.Unlock()
	s.sessionMU.Lock()
	if s.session != nil {
		s.session.Close()
	}
	s.session = nil
	s.sessionMU.Unlock()
	select {
	case stop := <-s.stopper:
		stop()
	default:
	}
	return
}

type Notifier struct {
	sdk      *SDK
	halt     chan struct{}
	haltOnce *sync.Once
}

// Get get a event from sdk, should be called in a loop
func (n *Notifier) Get() (event Event, err error) {
	select {
	case event = <-n.sdk.events:
	case <-n.halt:
		err = fmt.Errorf("notifier closed")
	}
	return
}

func (n *Notifier) Close() (err error) {
	n.haltOnce.Do(func() { close(n.halt) })
	return
}

type Session struct {
	sdk           *SDK
	peerID        peer.ID
	statusUpdates <-chan service.ProbeStatus
	halt          chan struct{}
	haltOnce      *sync.Once
}

// Fetch fetch last historical messages with offset and limit,
// call Message.Next() to get next message in the returned messages,
// if Message.Next() return null, the end of the messages has been reached
func (s *Session) Fetch(offset int, limit int) (messages *Message) {
	data, _ := s.sdk.getClient().FindLastMessages(s.peerID, offset, limit)
	var message *Message
	for _, val := range data {
		if messages == nil {
			message = &Message{
				PeerID:    s.peerID.String(),
				ID:        strconv.FormatUint(val.ID, 10),
				Type:      val.Type,
				Payload:   val.Payload,
				Reached:   val.Reached,
				Initiator: val.Initiator,
				CreatedAt: val.CreatedAt.UnixMilli(),
				TTL:       val.TTL.Milliseconds(),
			}
			messages = message
		} else {
			message.next = &Message{
				PeerID:    s.peerID.String(),
				ID:        strconv.FormatUint(val.ID, 10),
				Type:      val.Type,
				Payload:   val.Payload,
				Reached:   val.Reached,
				Initiator: val.Initiator,
				CreatedAt: val.CreatedAt.UnixMilli(),
				TTL:       val.TTL.Milliseconds(),
			}
			message = message.next
		}
	}
	return
}

// Send the ID of Message object will be updated if it is sent successfully
func (s *Session) Send(message *Message) (err error) {
	var mid uint64
	switch message.Type {
	case MsgTypeText:
		ttl := time.Duration(message.TTL) * time.Millisecond
		mid, err = s.sdk.getClient().SendText(s.peerID, string(message.Payload), ttl)
		if err != nil {
			err = fmt.Errorf("failed to send text: %v", err)
			return
		}
	case MsgTypeImage:
		mid, err = s.sdk.getClient().SendImage(s.peerID, message.Payload)
		if err != nil {
			err = fmt.Errorf("failed to send image: %v", err)
			return
		}
	default:
		err = fmt.Errorf("unkonwn message type")
		return
	}
	msg, err := s.sdk.getClient().FindMessage(s.peerID, mid)
	if err != nil {
		err = fmt.Errorf("failed to find message: %v", err)
		return
	}
	message.ID = strconv.FormatUint(msg.ID, 10)
	message.Type = msg.Type
	message.Payload = msg.Payload
	message.Reached = msg.Reached
	message.Initiator = msg.Initiator
	message.CreatedAt = msg.CreatedAt.UnixMilli()
	message.TTL = msg.TTL.Milliseconds()
	return
}

// Status get the connection status with the target user peer, should be called in a loop too
func (s *Session) Status() (status byte, err error) {
RETRY:
	select {
	case stat, ok := <-s.statusUpdates:
		if ok {
			status = byte(stat)
		} else {
			s.statusUpdates = s.sdk.getClient().ProbeStatus(s.peerID)
			goto RETRY
		}
	case <-s.halt:
		err = fmt.Errorf("session closed")
	}
	return
}

// Delete delete dialogue message by the id of message object.
// you can embed multiple message objects through calling append method,
// and pass them in to delete multiple messages.
func (s *Session) Delete(messages *Message) (err error) {
	var mids []uint64
	for message := messages; message != nil; message = message.Next() {
		id, e := strconv.ParseUint(message.ID, 10, 64)
		if e != nil {
			err = fmt.Errorf("failed to parse message id: %v", e)
			return
		}
		mids = append(mids, id)
	}
	err = s.sdk.getClient().DeleteMessages(s.peerID, mids)
	return
}

func (s *Session) Close() (err error) {
	s.haltOnce.Do(func() { close(s.halt) })
	return
}
