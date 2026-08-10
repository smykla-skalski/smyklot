package main

import (
	"context"
	"time"
)

const (
	panelMaintenanceInterval = 5 * time.Minute
	deliveryRetention        = 30 * 24 * time.Hour
)

func (s *server) panelMaintenanceLoop(ctx context.Context) {
	s.maintainPanel(ctx)
	ticker := time.NewTicker(panelMaintenanceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.maintainPanel(ctx)
		}
	}
}

func (s *server) maintainPanel(ctx context.Context) {
	if err := s.refreshPanelCatalog(ctx); err != nil {
		s.logger.Error("panel catalog synchronization failed", "error", err)
	}
	now := time.Now().UTC()
	if err := s.store.DeleteExpiredAuth(ctx, now); err != nil {
		s.logger.Error("panel authentication cleanup failed", "error", err)
	}
	if err := s.store.PruneDeliveries(ctx, now.Add(-deliveryRetention)); err != nil {
		s.logger.Error("panel delivery cleanup failed", "error", err)
	}
}

func (s *server) refreshPanelCatalog(ctx context.Context) error {
	_, err := s.SyncCatalog(ctx)

	return err
}
