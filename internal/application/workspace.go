package application

import (
	"context"
	"time"
)

type Workspace struct {
	Catalog   Catalog             `json:"catalog"`
	Lookups   map[string][]string `json:"lookups"`
	Favorites []Object            `json:"favorites"`
	Policies  []Policy            `json:"policies"`
}

// GetWorkspace reads related UI data from one authorized repeatable-read snapshot.
func (s *Service) GetWorkspace() (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return Workspace{}, err
	}
	defer tx.Rollback(ctx)
	var out Workspace
	if out.Catalog, err = loadCatalog(ctx, tx); err != nil {
		return Workspace{}, err
	}
	if out.Lookups, err = loadLookups(ctx, tx); err != nil {
		return Workspace{}, err
	}
	if out.Favorites, err = loadFavorites(ctx, tx, s.user.ID); err != nil {
		return Workspace{}, err
	}
	if out.Policies, err = loadPolicies(ctx, tx); err != nil {
		return Workspace{}, err
	}
	return out, nil
}
