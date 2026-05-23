package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/abhinavxd/libredesk/internal/realtime"
	"github.com/abhinavxd/libredesk/internal/ws"
	wsmodels "github.com/abhinavxd/libredesk/internal/ws/models"
	"github.com/redis/go-redis/v9"
	"github.com/zerodha/logf"
)

func startRealtimeBridge(ctx context.Context, rdb *redis.Client, wsHub *ws.Hub, lo *logf.Logger) {
	if rdb == nil || wsHub == nil {
		return
	}
	sub, err := realtime.NewRedisSubscriber(rdb, lo)
	if err != nil {
		lo.Warn("realtime bridge disabled", "error", err)
		return
	}
	go func() {
		if err := sub.Run(ctx, func(_ context.Context, event realtime.Event) error {
			payload, ok, err := realtimeWebSocketPayload(event)
			if err != nil {
				return err
			}
			if !ok {
				lo.Debug("realtime: ignoring unknown event type", "type", event.Type)
				return nil
			}
			wsHub.BroadcastMessage(wsmodels.BroadcastMessage{Data: payload})
			return nil
		}); err != nil && !errors.Is(err, context.Canceled) {
			lo.Error("realtime bridge stopped", "error", err)
		}
	}()
}

func realtimeWebSocketPayload(event realtime.Event) ([]byte, bool, error) {
	switch event.Type {
	case realtime.EventConversationUpdate:
		var data map[string]any
		if err := json.Unmarshal(event.Data, &data); err != nil {
			return nil, false, fmt.Errorf("decode conversation update: %w", err)
		}
		if data["uuid"] == nil {
			return nil, false, fmt.Errorf("conversation update missing uuid")
		}
		payload, err := json.Marshal(wsmodels.Message{
			Type: wsmodels.MessageTypeConversationUpdate,
			Data: data,
		})
		return payload, true, err
	case realtime.EventAgentValidationCompleted:
		var data map[string]any
		if err := json.Unmarshal(event.Data, &data); err != nil {
			return nil, false, fmt.Errorf("decode agent validation event: %w", err)
		}
		if data["conversation_uuid"] == nil {
			return nil, false, fmt.Errorf("agent validation event missing conversation_uuid")
		}
		payload, err := json.Marshal(wsmodels.Message{
			Type: wsmodels.MessageTypeAgentValidationDone,
			Data: data,
		})
		return payload, true, err
	default:
		return nil, false, nil
	}
}
