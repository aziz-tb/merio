package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"

	"github.com/aziz-tb/merio/internal/ai/runner"
	runnerevents "github.com/aziz-tb/merio/internal/ai/runner/events"
	"github.com/aziz-tb/merio/internal/envelope"
)

func handleAgentRunStream(r *fastglue.Request) error {
	app := r.Context.(*App)
	if app.temporal == nil || app.redis == nil {
		return r.SendErrorEnvelope(fasthttp.StatusServiceUnavailable,
			"AI run streaming is not configured", nil, envelope.GeneralError)
	}

	traceID, _ := r.RequestCtx.UserValue("trace_id").(string)
	if _, err := uuid.Parse(traceID); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest,
			"invalid trace_id", nil, envelope.InputError)
	}

	descCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := ensureWorkflowExists(descCtx, app.temporal, traceID); err != nil {
		if isTemporalNotFound(err) {
			return r.SendErrorEnvelope(fasthttp.StatusNotFound,
				"agent run workflow not found", nil, envelope.InputError)
		}
		app.lo.Error("failed to describe agent run workflow",
			"trace_id", traceID,
			"error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError,
			app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	startID := string(r.RequestCtx.Request.Header.Peek("Last-Event-ID"))
	if startID == "" {
		startID = "0-0"
	}
	keepalive := sseKeepaliveInterval()
	key := runnerevents.StreamKey(traceID)

	r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
	r.RequestCtx.Response.Header.Set("Content-Type", "text/event-stream")
	r.RequestCtx.Response.Header.Set("Cache-Control", "no-cache")
	r.RequestCtx.Response.Header.Set("Connection", "keep-alive")
	r.RequestCtx.Response.Header.Set("X-Accel-Buffering", "no")
	r.RequestCtx.SetBodyStreamWriter(func(w *bufio.Writer) {
		ctx := context.Background()
		lastID := startID
		lastKeepalive := time.Now()
		for {
			streams, err := app.redis.XRead(ctx, &redis.XReadArgs{
				Streams: []string{key, lastID},
				Block:   5 * time.Second,
				Count:   100,
			}).Result()
			if err != nil {
				if errors.Is(err, redis.Nil) {
					if time.Since(lastKeepalive) >= keepalive {
						if _, writeErr := w.WriteString(":keepalive\n\n"); writeErr != nil {
							return
						}
						if flushErr := w.Flush(); flushErr != nil {
							return
						}
						lastKeepalive = time.Now()
					}
					continue
				}
				return
			}
			for _, stream := range streams {
				for _, msg := range stream.Messages {
					raw, ok := msg.Values["json"]
					if !ok {
						continue
					}
					payload := fmt.Sprint(raw)
					var ev runner.RunEvent
					if err := json.Unmarshal([]byte(payload), &ev); err != nil {
						continue
					}
					if _, err := fmt.Fprintf(w, "id: %s\n", msg.ID); err != nil {
						return
					}
					if _, err := fmt.Fprintf(w, "event: %s\n", ev.Type); err != nil {
						return
					}
					if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
						return
					}
					if err := w.Flush(); err != nil {
						return
					}
					lastID = msg.ID
					lastKeepalive = time.Now()
					if ev.Type == runner.EventTypeRunCompleted || ev.Type == runner.EventTypeRunFailed {
						return
					}
				}
			}
		}
	})
	return nil
}

func sseKeepaliveInterval() time.Duration {
	if d := ko.Duration("agent_run.sse_keepalive"); d > 0 {
		return d
	}
	return 15 * time.Second
}
