package websocket

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Message types for WebSocket communication
const (
	MessageTypeFocusStarted   = "focus_started"
	MessageTypeFocusStopped   = "focus_stopped"
	MessageTypeLeaderboardUpdate = "leaderboard_update"
)

// WebSocket message structure
type WSMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// FocusUpdate payload for live focus notifications
type FocusUpdate struct {
	UserID       string    `json:"user_id"`
	Pseudo       string    `json:"pseudo"`
	AvatarURL    *string   `json:"avatar_url"`
	IsLive       bool      `json:"is_live"`
	StartedAt    *string   `json:"started_at,omitempty"`
	DurationMins *int      `json:"duration_mins,omitempty"`
}

// Client represents a WebSocket connection
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	userID string
}

// Hub maintains active WebSocket connections
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mutex      sync.RWMutex
}

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for now (configure for production)
		},
	}

	// Global hub instance
	GlobalHub *Hub
)

// NewHub creates a new Hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mutex.Lock()
			h.clients[client] = true
			h.mutex.Unlock()
			fmt.Printf("🔌 WebSocket client connected: %s (total: %d)\n", client.userID, len(h.clients))

		case client := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mutex.Unlock()
			fmt.Printf("🔌 WebSocket client disconnected: %s (total: %d)\n", client.userID, len(h.clients))

		case message := <-h.broadcast:
			h.mutex.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mutex.RUnlock()
		}
	}
}

// BroadcastFocusUpdate sends a focus update to all connected clients
func (h *Hub) BroadcastFocusUpdate(update FocusUpdate) {
	msg := WSMessage{
		Type:    MessageTypeFocusStarted,
		Payload: update,
	}
	if !update.IsLive {
		msg.Type = MessageTypeFocusStopped
	}

	data, err := json.Marshal(msg)
	if err != nil {
		fmt.Println("Error marshaling focus update:", err)
		return
	}

	h.broadcast <- data
	fmt.Printf("📡 Broadcast focus update: user=%s, isLive=%v\n", update.UserID, update.IsLive)
}

// ServeWs handles WebSocket requests
func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request, userID string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("WebSocket upgrade error:", err)
		return
	}

	client := &Client{
		hub:    hub,
		conn:   conn,
		send:   make(chan []byte, 256),
		userID: userID,
	}
	client.hub.register <- client

	// Start goroutines for reading and writing
	go client.writePump()
	go client.readPump()
}

// readPump pumps messages from the WebSocket connection to the hub
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				fmt.Printf("WebSocket error: %v\n", err)
			}
			break
		}
	}
}

// writePump pumps messages from the hub to the WebSocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// InitGlobalHub initializes and starts the global WebSocket hub
func InitGlobalHub() {
	GlobalHub = NewHub()
	go GlobalHub.Run()
	fmt.Println("🚀 WebSocket hub started")
}
