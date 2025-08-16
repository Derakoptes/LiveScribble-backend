# LiveScribble Backend

LiveScribble is a collaborative document editing platform (Google Docs–style) with real-time updates, live cursor presence, and link-based sharing.  
This repository contains the **backend service**, written in Go.

---

## 🚀 Features (Current Stage)

- **User Authentication** – basic auth layer implemented.  
- **Document Fetching (REST)** – retrieve all documents or retrieve a document by ID.  
- **WebSocket Collaboration** – upgrade connections to WebSocket for real-time sync.  
- **Event Frames** – structured binary/JSON messages for CRDT updates and presence.

---

## 📡 Protocol Frames

The backend uses a **framed WebSocket protocol**:

| Frame | Code | Payload Type | Description |
|-------|------|--------------|-------------|
| `FrameUpdate` | `0x01` | Binary | Incremental CRDT update |
| `FrameSnapshot` | `0x02` | Binary | Full document snapshot |
| `FrameAwareness` | `0x10` | JSON | User presence (cursor, name, color, etc.) |
| `FrameControl` | `0x11` | JSON | Control messages (join/leave) |
| `FrameRequestSnap` | `0x20` | Server → Client | Request snapshot |
| `FrameSnapshotUpdateFailed` | `0x21` | JSON | Snapshot update failed |
| `FrameSnapshotUpdateSuccess` | `0x22` | JSON | Snapshot update succeeded |

---

## 🛠️ Tech Stack

- **Go** – backend language  
- **Gin** – HTTP router for REST endpoints  
- **Gorilla WebSocket** – real-time collaboration  
- **Redis)** – Pub/Sub for horizontal scaling (not yet integrated)

---

