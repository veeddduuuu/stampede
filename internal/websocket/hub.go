package websocket

// Hub maintains the set of active clients and broadcasts messages to the clients.
type Hub struct {
	// Registered clients map (client pointer -> true)
	clients map[*Client]bool

	// Inbound messages from application/Redis to broadcast to all clients
	broadcast chan []byte

	// Register requests from connecting clients
	register chan *Client

	// Unregister requests from disconnected clients
	unregister chan *Client
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run starts the hub's event loop.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Channel buffer is full (slow/dead client). Drop client.
					close(client.send)
					delete(h.clients, client)
				}
			}
		}
	}
}
