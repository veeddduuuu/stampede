package websocket

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"

)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow CORS connections for WebSocket handshake
		return true
	},
}

// ServeWS handles websocket requests from the peer.
func ServeWS(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}

	client := NewClient(hub, conn)
	client.hub.register <- client

	// Allow collection of memory referenced by the caller by doing all work in new goroutines.
	go client.WritePump()
	go client.ReadPump()
}
