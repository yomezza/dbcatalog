package application

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
)

type Change struct {
	Schema string `json:"schema"`
	Type   string `json:"type"`
}
type liveState struct {
	cancel     context.CancelFunc
	generation atomic.Uint64
	handler    func(Change)
}

// SetChangeHandler connects the existing desktop process to commit notifications.
// The callback must not block; no application data or credential is in its payload.
func (s *Service) SetChangeHandler(handler func(Change)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.live.handler = handler
	s.startLiveLocked()
}
func (s *Service) stopLiveLocked() {
	s.live.generation.Add(1)
	if s.live.cancel != nil {
		s.live.cancel()
		s.live.cancel = nil
	}
}
func (s *Service) startLiveLocked() {
	s.stopLiveLocked()
	if s.pool == nil || s.live.handler == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.live.cancel = cancel
	cfg := s.pool.Config().ConnConfig.Copy()
	cfg.RuntimeParams["application_name"] = "CatalogDB.Listener:" + s.schema
	schema := s.schema
	handler := s.live.handler
	generation := s.live.generation.Load()
	go func() {
		delay := time.Second
		emit := func(c Change) {
			if ctx.Err() == nil && s.live.generation.Load() == generation {
				handler(c)
			}
		}
		for ctx.Err() == nil {
			conn, err := pgx.ConnectConfig(ctx, cfg)
			if err == nil {
				_, err = conn.Exec(ctx, "LISTEN catalogdb_events")
				if err == nil {
					delay = time.Second
					emit(Change{schema, "resync"})
					for ctx.Err() == nil {
						notification, e := conn.WaitForNotification(ctx)
						if e != nil {
							break
						}
						var change Change
						if json.Unmarshal([]byte(notification.Payload), &change) == nil && change.Schema == schema &&
							(change.Type == "catalog_changed" || change.Type == "users_changed" || change.Type == "notifications_changed") {
							emit(change)
						}
					}
				}
				closeCtx, closeCancel := context.WithTimeout(context.Background(), time.Second)
				_ = conn.Close(closeCtx)
				closeCancel()
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if delay < 30*time.Second {
				delay *= 2
				if delay > 30*time.Second {
					delay = 30 * time.Second
				}
			}
		}
	}()
}
