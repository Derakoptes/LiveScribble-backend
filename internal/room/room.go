package room

import (
	"livescribble/internal/utils"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
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

type Room struct {
	logger *slog.Logger

	docId string

	db       *gorm.DB
	clients  map[*websocket.Conn]bool
	clientMu sync.RWMutex

	onEmpty func(string)
}

func NewRoom(docId string, db *gorm.DB) *Room {
	return &Room{
		docId:   docId,
		db:      db,
		clients: make(map[*websocket.Conn]bool),
	}
}

func (r *Room) SetOnEmptyCallback(callback func(string)) {
	r.onEmpty = callback
}

func (r *Room) AddClient(c *websocket.Conn) {
	r.clientMu.Lock()
	r.clients[c] = true
	r.clientMu.Unlock()

	r.listenToClient(c)
}

func (r *Room) listenToClient(c *websocket.Conn) {
	defer func() {
		r.removeClient(c)
	}()

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

			if len(data) > 0 && data[0] == FrameSnapshot {
				payload := data[1:]
				if err := r.saveSnapshot(payload); err != nil {
					r.logger.Error("saving snapshot failed", "error", r.docId)
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
	delete(r.clients, c)
	_ = c.Close()

	if len(r.clients) == 0 && r.onEmpty != nil {
		go r.onEmpty(r.docId)
	}

	r.clientMu.Unlock()
}
