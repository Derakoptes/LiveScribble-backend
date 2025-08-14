package room

import (
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

type RoomManager struct {
	logger *slog.Logger
	db     *gorm.DB
	rooms  map[string]*Room
	roomMu sync.RWMutex
}

func NewRoomManager(db *gorm.DB, logger *slog.Logger) *RoomManager {
	rm := &RoomManager{
		logger: logger,
		db:     db,
		rooms:  make(map[string]*Room),
	}

	go rm.startPeriodicSnapshotRequests()

	return rm
}

func (rm *RoomManager) JoinRoom(docId string, conn *websocket.Conn) {
	rm.roomMu.Lock()
	defer rm.roomMu.Unlock()

	room, exists := rm.rooms[docId]
	if !exists {
		room = NewRoom(docId, rm.db)
		room.logger = rm.logger
		room.SetOnEmptyCallback(rm.RemoveRoom)
		rm.rooms[docId] = room
		rm.logger.Info("Created new room", "docId", docId)
	}

	go room.AddClient(conn)
	rm.logger.Info("Client joined room", "docId", docId)
}

func (rm *RoomManager) RemoveRoom(docId string) {
	rm.roomMu.Lock()
	defer rm.roomMu.Unlock()

	if room, exists := rm.rooms[docId]; exists {
		// Check if room has no clients
		room.clientMu.RLock()
		clientCount := len(room.clients)
		room.clientMu.RUnlock()

		if clientCount == 0 {
			delete(rm.rooms, docId)
			rm.logger.Info("Removed empty room", "docId", docId)
		}
	}
}

func (rm *RoomManager) GetRoomCount() int {
	rm.roomMu.RLock()
	defer rm.roomMu.RUnlock()
	return len(rm.rooms)
}

func (rm *RoomManager) startPeriodicSnapshotRequests() {
	ticker := time.NewTicker(30 * time.Second) // Request snapshots every 30 seconds
	defer ticker.Stop()

	for range ticker.C {
		rm.roomMu.RLock()
		for docId, room := range rm.rooms {
			room.clientMu.RLock()
			clientCount := len(room.clients)
			room.clientMu.RUnlock()

			if clientCount > 0 {
				rm.logger.Debug("Requesting snapshot from clients", "docId", docId, "clientCount", clientCount)
				room.requestSnapshotFromClients()
			} else {
				go func(id string) {
					time.Sleep(5 * time.Second)
					rm.RemoveRoom(id)
				}(docId)
			}
		}
		rm.roomMu.RUnlock()
	}
}
