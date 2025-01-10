package main

import (
	"bufio"
	"chat-libp2p/cmd/client/mobile"
	"chat-libp2p/pkg/logger"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/libp2p/go-libp2p/core/peer"
)

func init() {
	var out io.Writer
	defer func() {
		if out == nil {
			logger.RegisterLogger(0, "", io.Discard)
		} else {
			logger.RegisterLogger(log.LstdFlags|log.Lmicroseconds, "", out)
		}
	}()
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	logFile := filepath.Join(filepath.Dir(exePath), "aks_sdk.log")
	// TODO make it rolling
	if fstat, err := os.Stat(logFile); err != nil || fstat.Size() > 1024*1024*10 { // limit log file size to 10MB
		out, _ = os.Create(logFile)
	} else {
		out, _ = os.OpenFile(logFile, os.O_RDWR|os.O_APPEND, os.ModePerm)
	}
}

func main() {
	var (
		reader  = bufio.NewReader(os.Stdin)
		message = &Message{}
		tPeerID peer.ID
		session *mobile.Session
		running = &atomic.Bool{}
	)
	logger.Info("sdk process launched")
COMMAND:
	for {
		command, _, _ := reader.ReadLine()
		message.Reset()
		err := message.Decode(command)
		if err != nil || message.Type != TypeCommand {
			message.Response(CodeIllegal, err)
			continue COMMAND
		} else if message.Label != CommandInit && !running.Load() {
			message.Response(CodeIllegal, fmt.Errorf("sdk not initialized"))
			continue COMMAND
		}
		logger.Info("sdk command: %s", message.Label)
		switch message.Label {
		case CommandInit:
			confBuf, err := json.Marshal(message.Payload)
			if err != nil {
				message.Response(CodeIllegal, err)
				continue COMMAND
			}
			if err = mobile.GetSDK().Init(confBuf); err != nil {
				message.Response(CodeFailure, err)
				continue COMMAND
			}
			peerID := mobile.GetSDK().GetPeerID()
			peerInfo, err := mobile.GetSDK().GetPeerInfo(peerID)
			if err != nil {
				message.Response(CodeFailure, err)
				continue COMMAND
			}
			notifier := mobile.GetSDK().GetNotifier()
			go func(notifier *mobile.Notifier) {
				event := &Message{}
				for {
					e, err := notifier.Get()
					if err != nil {
						logger.Error("failed to receive message: %v", err)
						return
					}
					switch v := e.(type) {
					case *mobile.Arrived:
						event.Notify(EventArrived, v)
					case *mobile.Message:
						event.Notify(EventReceived, v)
					}
				}
			}(notifier)
			running.Store(true)
			message.Response(CodeSuccess, &ResponsePayloadPeerAddr{&PeerID{PeerID: peerID}, peerInfo.EVMAddr})
			continue COMMAND
		case CommandPeers:
			peers := make([]*mobile.Peer, 0)
			for peer, _ := mobile.GetSDK().GetPeerList(); peer != nil; peer = peer.Next() {
				peers = append(peers, peer)
			}
			message.Response(CodeSuccess, peers)
			continue COMMAND
		case CommandPin:
			payload := &RequestPayloadPin{}
			message.Payload = payload
			message.Decode(command)
			message.Payload = nil
			if payload.Pin {
				err := mobile.GetSDK().PinSession(payload.PeerID.PeerID)
				if err != nil {
					message.Response(CodeFailure, err)
					continue COMMAND
				}
			} else {
				err := mobile.GetSDK().UnpinSession(payload.PeerID.PeerID)
				if err != nil {
					message.Response(CodeFailure, err)
					continue COMMAND
				}
			}
		case CommandRemark:
			payload := &RequestPayloadRemark{}
			message.Payload = payload
			message.Decode(command)
			message.Payload = nil
			err := mobile.GetSDK().SetRemark(payload.PeerID.PeerID, payload.Remark)
			if err != nil {
				message.Response(CodeFailure, err)
				continue COMMAND
			}
		case CommandFlush:
			payload := &PeerID{}
			message.Payload = payload
			message.Decode(command)
			message.Payload = nil
			err := mobile.GetSDK().DeleteDialogue(payload.PeerID)
			if err != nil {
				message.Response(CodeFailure, err)
				continue COMMAND
			}
		case CommandOpen:
			payload := &PeerID{}
			message.Payload = payload
			message.Decode(command)
			message.Payload = nil
			session, err = mobile.GetSDK().OpenSession(payload.PeerID)
			if err != nil {
				message.Response(CodeFailure, err)
				continue COMMAND
			}
			tPeerID, _ = peer.Decode(payload.PeerID)
			go func(session *mobile.Session) {
				event := &Message{}
				for {
					status, err := session.Status()
					if err != nil {
						logger.Error("failed to monitor session status: %v", err)
						return
					}
					switch status {
					case mobile.StatusOff:
						event.Notify(EventStatus, &EventStatusPayload{Status: StatusOff})
					case mobile.StatusRelay:
						event.Notify(EventStatus, &EventStatusPayload{Status: StatusRelay})
					case mobile.StatusDirect:
						event.Notify(EventStatus, &EventStatusPayload{Status: StatusDirect})
					}
				}
			}(session)
		case CommandFetch:
			payload := &RequestPayloadRange{}
			message.Payload = payload
			message.Decode(command)
			message.Payload = nil
			peerID, err := peer.Decode(payload.PeerID.PeerID)
			if err != nil {
				message.Response(CodeIllegal, err)
				continue COMMAND
			} else if peerID != tPeerID || session == nil {
				message.Response(CodeFailure, fmt.Errorf("should open %s first", peerID))
				continue COMMAND
			}
			messages := make([]*mobile.Message, 0)
			for message := session.Fetch(payload.Offset, payload.Limit); message != nil; message = message.Next() {
				messages = append(messages, message)
			}
			message.Response(CodeSuccess, messages)
			continue COMMAND
		case CommandDelete:
			payload := &RequestPayloadDelete{}
			message.Payload = payload
			message.Decode(command)
			message.Payload = nil
			peerID, err := peer.Decode(payload.PeerID.PeerID)
			if err != nil {
				message.Response(CodeIllegal, err)
				continue COMMAND
			} else if peerID != tPeerID || session == nil {
				message.Response(CodeFailure, fmt.Errorf("should open %s first", peerID))
				continue COMMAND
			}
			param := &mobile.Message{}
			curr := param
			for i, messageID := range payload.MessageIDs {
				if i == 0 {
					param.ID = messageID
				} else {
					m := &mobile.Message{ID: messageID}
					curr.Append(m)
					curr = m
				}
			}
			if err = session.Delete(param); err != nil {
				message.Response(CodeFailure, err)
				continue COMMAND
			}
		case CommandSend:
			payload := &mobile.Message{}
			message.Payload = payload
			message.Decode(command)
			message.Payload = nil
			peerID, err := peer.Decode(payload.PeerID)
			if err != nil {
				message.Response(CodeIllegal, err)
				continue COMMAND
			} else if peerID != tPeerID || session == nil {
				message.Response(CodeFailure, fmt.Errorf("should open %s first", peerID))
				continue COMMAND
			}
			if err = session.Send(payload); err != nil {
				message.Response(CodeFailure, err)
				continue COMMAND
			}
			message.Response(CodeSuccess, payload)
			continue COMMAND
		case CommandClose:
			tPeerID = ""
			session = nil
			mobile.GetSDK().Close()
			running.Store(false)
		}
		message.Response(CodeSuccess)
	}
}

const (
	TypeCommand  string = "command"
	TypeResponse string = "response"
	TypeEvent    string = "event"
)

const (
	CodeSuccess int = 1
	CodeFailure int = -1
	CodeIllegal int = -2
)

const (
	CommandInit   string = "init"
	CommandPeers  string = "peers"
	CommandPin    string = "pin_peer"
	CommandRemark string = "set_remark"
	CommandFlush  string = "delete_dialogue"
	CommandOpen   string = "open"
	CommandFetch  string = "fetch_history"
	CommandDelete string = "delete_message"
	CommandSend   string = "send"
	CommandClose  string = "close"
)

const (
	StatusOff    string = "off"
	StatusRelay  string = "relay"
	StatusDirect string = "direct"
)

const (
	EventStatus   string = "peer_status"
	EventArrived  string = "message_arrived"
	EventReceived string = "message_received"
)

type Message struct {
	Type    string `json:"type"`
	Txid    int    `json:"txid,omitempty"`
	Code    int    `json:"code,omitempty"`
	Label   string `json:"label,omitempty"`
	Payload any    `json:"payload,omitempty"`
}

func (m *Message) Decode(buf []byte) (err error) {
	err = json.Unmarshal(buf, m)
	return
}

func (m *Message) String() string {
	buf, _ := json.Marshal(m)
	return string(buf)
}

func (m *Message) Reset() {
	m.Type = ""
	m.Txid = 0
	m.Code = 0
	m.Label = ""
	m.Payload = nil
}

func (m *Message) Response(code int, payload ...any) {
	m.Type = TypeResponse
	m.Code = code
	m.Label = ""
	m.Payload = nil
	if len(payload) > 0 {
		switch v := payload[0].(type) {
		case error:
			m.Payload = v.Error()
		default:
			m.Payload = payload[0]
		}
	}
	fmt.Println(m)
}

func (m *Message) Notify(label string, payload ...any) {
	m.Type = TypeEvent
	m.Txid = 0
	m.Code = 0
	m.Label = label
	m.Payload = nil
	if len(payload) > 0 {
		m.Payload = payload[0]
	}
	fmt.Println(m)
}

type PeerID struct {
	PeerID string `json:"peer_id"`
}

type RequestPayloadPin struct {
	*PeerID
	Pin bool `json:"pin"`
}

type RequestPayloadRemark struct {
	*PeerID
	Remark string `json:"remark"`
}

type RequestPayloadRange struct {
	*PeerID
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

type RequestPayloadDelete struct {
	*PeerID
	MessageIDs []string `json:"message_ids"`
}

type ResponsePayloadPeerAddr struct {
	*PeerID
	EVMAddr string `json:"evm_addr"`
}

type EventStatusPayload struct {
	Status string `json:"status"`
}
