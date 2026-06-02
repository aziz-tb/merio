// Package ws handles WebSocket connections and broadcasting messages to clients.
package ws

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/aziz-tb/merio/internal/ws/models"
	"github.com/fasthttp/websocket"
)

// ViewerReconcileInterval is the period at which the hub re-broadcasts the per-conversation
// viewer list. This is option-(a) reconciliation: it self-heals stale viewers if a disconnect
// path was missed (e.g. the write-pump error path that returns without RemoveClient).
const ViewerReconcileInterval = 30 * time.Second

// Hub maintains the set of registered websockets clients.
type Hub struct {
	// Client ID to WS Client map, user can connect from multiple devices and each device will have a separate client.
	clients      map[int][]*Client
	clientsMutex sync.RWMutex

	// Conversation UUID to clients map for faster conversation broadcasting
	conversationClients      map[string][]*Client
	conversationClientsMutex sync.RWMutex

	userStore         userStore
	conversationStore conversationStore
}

type userStore interface {
	UpdateLastActive(userID int) (bool, error)
}

type conversationStore interface {
	BroadcastTypingToWidgetClientsOnly(conversationUUID string, isTyping bool)
}

// NewHub creates a new websocket hub.
func NewHub(userStore userStore) *Hub {
	return &Hub{
		clients:                  make(map[int][]*Client, 10000),
		clientsMutex:             sync.RWMutex{},
		conversationClients:      make(map[string][]*Client),
		conversationClientsMutex: sync.RWMutex{},
		userStore:                userStore,
		// To be set later via conversationStore.
		conversationStore: nil,
	}
}

// SetConversationStore sets the conversation store for cross-broadcasting.
func (h *Hub) SetConversationStore(manager conversationStore) {
	h.conversationStore = manager
}

// AddClient adds a new client to the hub.
func (h *Hub) AddClient(client *Client) {
	h.clientsMutex.Lock()
	defer h.clientsMutex.Unlock()
	h.clients[client.ID] = append(h.clients[client.ID], client)
}

// RemoveClient removes a client from the hub. After removal, viewer-presence is broadcast for
// every conversation the client was subscribed to, so the remaining viewers see the departure.
func (h *Hub) RemoveClient(client *Client) {
	h.clientsMutex.Lock()
	defer h.clientsMutex.Unlock()

	// Capture which conversations the client was in (for post-unlock broadcast),
	// then remove from all conversation subscriptions.
	h.conversationClientsMutex.Lock()
	affected := h.findClientConversationsLocked(client)
	h.removeClientFromAllConversations(client)
	h.conversationClientsMutex.Unlock()

	if clients, ok := h.clients[client.ID]; ok {
		for i, c := range clients {
			if c == client {
				h.clients[client.ID] = append(clients[:i], clients[i+1:]...)
				break
			}
		}
	}

	// Broadcast viewer updates outside the locks.
	for _, uuid := range affected {
		h.BroadcastConversationViewers(uuid)
	}
}

// BroadcastMessage broadcasts a message to the specified users.
// If no users are specified, the message is broadcast to all users.
func (h *Hub) BroadcastMessage(msg models.BroadcastMessage) {
	h.clientsMutex.RLock()
	defer h.clientsMutex.RUnlock()

	// Broadcast to all users if no users are specified.
	if len(msg.Users) == 0 {
		for _, clients := range h.clients {
			for _, client := range clients {
				client.SendMessage(msg.Data, websocket.TextMessage)
			}
		}
		return
	}

	// Broadcast to specified users.
	for _, userID := range msg.Users {
		for _, client := range h.clients[userID] {
			client.SendMessage(msg.Data, websocket.TextMessage)
		}
	}
}

// SubscribeToConversation subscribes a client to a conversation. After (re)subscribing,
// viewer-presence is broadcast for both the new conversation and any conversation the client
// just left (so other viewers see them join the new room and leave the old one).
func (h *Hub) SubscribeToConversation(client *Client, conversationUUID string) {
	h.conversationClientsMutex.Lock()

	// Capture conversations the client was in before unsubscribing.
	previous := h.findClientConversationsLocked(client)

	// Unsubscribe from previous conversation if any
	h.removeClientFromAllConversations(client)

	// Subscribe to new conversation
	h.conversationClients[conversationUUID] = append(h.conversationClients[conversationUUID], client)

	h.conversationClientsMutex.Unlock()

	// Broadcast viewer updates outside the lock — the new room and any rooms the client left.
	h.BroadcastConversationViewers(conversationUUID)
	for _, uuid := range previous {
		if uuid != conversationUUID {
			h.BroadcastConversationViewers(uuid)
		}
	}
}

// findClientConversationsLocked returns the list of conversation UUIDs the given client is
// currently subscribed to. The caller must hold conversationClientsMutex.
func (h *Hub) findClientConversationsLocked(client *Client) []string {
	out := []string{}
	for uuid, clients := range h.conversationClients {
		for _, c := range clients {
			if c == client {
				out = append(out, uuid)
				break
			}
		}
	}
	return out
}

// GetConversationViewerUserIDs returns the deduped userIDs of clients currently subscribed to a
// conversation. A single user with multiple tabs / devices counts once.
func (h *Hub) GetConversationViewerUserIDs(conversationUUID string) []int {
	h.conversationClientsMutex.RLock()
	defer h.conversationClientsMutex.RUnlock()
	seen := map[int]struct{}{}
	out := []int{}
	for _, c := range h.conversationClients[conversationUUID] {
		if _, ok := seen[c.ID]; ok {
			continue
		}
		seen[c.ID] = struct{}{}
		out = append(out, c.ID)
	}
	return out
}

// BroadcastConversationViewers sends the deduped viewer-list to every client subscribed to the
// given conversation. Frontend treats each broadcast as authoritative (overwrites local state),
// so missed deltas can never leave stale viewers — option-(a) reconciliation.
func (h *Hub) BroadcastConversationViewers(conversationUUID string) {
	ids := h.GetConversationViewerUserIDs(conversationUUID)
	payload, err := json.Marshal(models.Message{
		Type: models.MessageTypeConversationViewers,
		Data: map[string]any{
			"conversation_uuid": conversationUUID,
			"user_ids":          ids,
		},
	})
	if err != nil {
		return
	}

	h.conversationClientsMutex.RLock()
	defer h.conversationClientsMutex.RUnlock()
	for _, client := range h.conversationClients[conversationUUID] {
		client.SendMessage(payload, websocket.TextMessage)
	}
}

// StartViewerReconciler runs a periodic re-broadcast of every active conversation's viewer list.
// This is the safety-net for the pre-existing write-pump disconnect-cleanup gap (Serve() in
// client.go can return without calling RemoveClient): a stale viewer drops within one tick.
// Cancel via the supplied context.
func (h *Hub) StartViewerReconciler(ctx context.Context) {
	go func() {
		t := time.NewTicker(ViewerReconcileInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				h.conversationClientsMutex.RLock()
				uuids := make([]string, 0, len(h.conversationClients))
				for uuid := range h.conversationClients {
					uuids = append(uuids, uuid)
				}
				h.conversationClientsMutex.RUnlock()
				for _, uuid := range uuids {
					h.BroadcastConversationViewers(uuid)
				}
			}
		}
	}()
}

// removeClientFromAllConversations removes a client from all conversation subscriptions.
// Must be called with conversationClientsMutex held.
func (h *Hub) removeClientFromAllConversations(client *Client) {
	for conversationUUID, clients := range h.conversationClients {
		for i, c := range clients {
			if c == client {
				h.conversationClients[conversationUUID] = append(clients[:i], clients[i+1:]...)
				if len(h.conversationClients[conversationUUID]) == 0 {
					delete(h.conversationClients, conversationUUID)
				}
				break
			}
		}
	}
}

// BroadcastTypingToConversation relays an agent's typing status to the customer widget only.
// Private-note typing is skipped.
func (h *Hub) BroadcastTypingToConversation(conversationUUID string, typingMsg models.TypingMessage) {
	if h.conversationStore != nil && !typingMsg.IsPrivateMessage {
		h.conversationStore.BroadcastTypingToWidgetClientsOnly(conversationUUID, typingMsg.IsTyping)
	}
}

// BroadcastTypingToAllConversationClients broadcasts typing status to all clients subscribed to a conversation.
func (h *Hub) BroadcastTypingToAllConversationClients(conversationUUID string, data []byte) {
	h.conversationClientsMutex.RLock()
	defer h.conversationClientsMutex.RUnlock()

	for _, client := range h.conversationClients[conversationUUID] {
		client.SendMessage(data, websocket.TextMessage)
	}
}
