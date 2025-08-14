package room

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"livescribble/internal/utils"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const (
	FrameUpdate      = 0x01 // CRDT binary update
	FrameSnapshot    = 0x02 // full snapshot (binary)
	FrameAwareness   = 0x10 // text JSON presence
	FrameControl     = 0x11 // join/leave etc. (JSON)
	FrameRequestSnap = 0x20 // server → client asks for snapshot

	FrameSnapshotUpdateFailed  = 0x21 //Snapshot update failed
	FrameSnapshotUpdateSuccess = 0x22 //Snapshot update success
)

type RedisMessage struct {
	Type     string `json:"type"`
	DocId    string `json:"docId"`
	Data     []byte `json:"data"`
	SenderId string `json:"senderId"` // connection ID to avoid echo
}

type Room struct {
	logger *slog.Logger

	docId string

	db          *gorm.DB
	redisClient *redis.Client
	ctx         context.Context
	cancelRedis context.CancelFunc

	clients  map[*websocket.Conn]string // map connection to connection ID
	clientMu sync.RWMutex

	onEmpty func(string)
}

func NewRoom(docId string, db *gorm.DB, redisClient *redis.Client) *Room {
	ctx, cancel := context.WithCancel(context.Background())

	r := &Room{
		docId:       docId,
		db:          db,
		redisClient: redisClient,
		ctx:         ctx,
		cancelRedis: cancel,
		clients:     make(map[*websocket.Conn]string),
	}

	go r.subscribeToRedis()

	return r
}

func (r *Room) SetOnEmptyCallback(callback func(string)) {
	r.onEmpty = callback
}

func (r *Room) AddClient(c *websocket.Conn) {
	r.clientMu.Lock()
	connId := generateConnectionId()
	r.clients[c] = connId
	r.clientMu.Unlock()

	r.listenToClient(c)
}

func (r *Room) listenToClient(c *websocket.Conn) {
	defer func() {
		r.removeClient(c)
	}()

	connId := r.clients[c]

	for {
		msgType, data, err := c.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				r.logger.Error("WebSocket error", "docId", r.docId, "error", err)
			}
			return
		}
		if msgType == websocket.BinaryMessage || msgType == websocket.TextMessage {
			r.broadcastLocal(data, c)

			r.broadcastToRedis(data, connId)

			if len(data) > 0 && data[0] == FrameSnapshot {
				payload := data[1:]
				if err := r.saveSnapshot(payload); err != nil {
        			r.logger.Error("Failed to save snapshot", "docId", r.docId, "error", err, "payloadSize", len(payload))
					errorMsg := []byte{FrameSnapshotUpdateFailed}
					r.broadcastToSingle(errorMsg, c)
					//in the frontend, ensure you await this message upon saving, to ensure that it has saved or not, also account for user spammign the save
				} else {
					r.broadcastToSingle([]byte{FrameSnapshotUpdateSuccess}, c)
				}
			}
		}
	}
}
func (r *Room) broadcastLocal(data []byte, sender *websocket.Conn) {
	r.clientMu.RLock()
	defer r.clientMu.RUnlock()

	for c := range r.clients {
		if c == sender {
			continue
		}
		_ = c.SetWriteDeadline(time.Now().Add(time.Second * 5))
		if err := c.WriteMessage(websocket.BinaryMessage, data); err != nil {
			r.removeClient(c)
		}
	}
}
func (r *Room) broadcastToSingle(data []byte, recipient *websocket.Conn) {
	r.clientMu.RLock()
	defer r.clientMu.RUnlock()

	_ = recipient.SetWriteDeadline(time.Now().Add(time.Second * 5))
	if err := recipient.WriteMessage(websocket.BinaryMessage, data); err != nil {
		r.removeClient(recipient)
	}
}
func (r *Room) requestSnapshotFromClients() {
	r.clientMu.RLock()
	defer r.clientMu.RUnlock()

	for c := range r.clients {
		_ = c.SetWriteDeadline(time.Now().Add(time.Second * 5))
		if err := c.WriteMessage(websocket.BinaryMessage, []byte{FrameRequestSnap}); err != nil {
			r.removeClient(c)
		}
	}
}
func (r *Room) saveSnapshot(payload []byte) error {
	return r.db.Model(&utils.Document{}).Where("id = ?", r.docId).Update("content", payload).Error
}

func (r *Room) removeClient(c *websocket.Conn) {
	r.clientMu.Lock()
	defer r.clientMu.Unlock()
	delete(r.clients, c)
	_ = c.Close()

	if len(r.clients) == 0 {
		// Cancel Redis subscription when room is empty
		r.cancelRedis()

		if r.onEmpty != nil {
			go r.onEmpty(r.docId)
		}
	}
}

func generateConnectionId() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func (r *Room) broadcastToRedis(data []byte, senderConnId string) {
	msg := RedisMessage{
		Type:     "broadcast",
		DocId:    r.docId,
		Data:     data,
		SenderId: senderConnId,
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		r.logger.Error("Failed to marshal Redis message", "error", err)
		return
	}

	channel := "room:" + r.docId
	if err := r.redisClient.Publish(r.ctx, channel, msgBytes).Err(); err != nil {
		r.logger.Error("Failed to publish to Redis", "error", err)
	}
}

func (r *Room) subscribeToRedis() {
	channel := "room:" + r.docId
	pubsub := r.redisClient.Subscribe(r.ctx, channel)
	defer pubsub.Close()

	ch := pubsub.Channel()

	for {
		select {
		case msg := <-ch:
			var redisMsg RedisMessage
			if err := json.Unmarshal([]byte(msg.Payload), &redisMsg); err != nil {
				r.logger.Error("Failed to unmarshal Redis message", "error", err)
				continue
			}

			// Don't broadcast back to local clients if this server sent it
			if redisMsg.Type == "broadcast" && !r.isLocalSender(redisMsg.SenderId){
				r.broadcastFromRedis(redisMsg.Data)
			}

		case <-r.ctx.Done():
			return
		}
	}
}

func (r *Room) isLocalSender(senderId string) bool {
    r.clientMu.RLock()
    defer r.clientMu.RUnlock()
    
    for _, connId := range r.clients {
        if connId == senderId {
            return true
        }
    }
    return false
}
func (r *Room) broadcastFromRedis(data []byte) {
	r.clientMu.RLock()
	defer r.clientMu.RUnlock()

	for c := range r.clients {
		_ = c.SetWriteDeadline(time.Now().Add(time.Second * 5))
		if err := c.WriteMessage(websocket.BinaryMessage, data); err != nil {
			go r.removeClient(c)
		}
	}
}
