package services

import (
	"context"
	"sort"

	"kuznica/api/domain"

	"github.com/google/uuid"
)

// Хранилище в памяти — методы контура автономии (agent_store.go).

func (m *memStore) ListAgentRunsByTrigger(_ context.Context, trigger string, limit int) ([]domain.AgentRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var res []domain.AgentRun
	for _, r := range m.runs {
		if trigger == "" || r.Trigger == trigger {
			res = append(res, r)
		}
	}
	sort.Slice(res, func(i, j int) bool { return res[i].StartedAt.After(res[j].StartedAt) })
	if len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

func (m *memStore) CreateSuppress(_ context.Context, s *domain.MemorySuppress) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ensureID(&s.UUIDModel)
	m.suppress = append(m.suppress, *s)
	return nil
}

func (m *memStore) SaveSuppress(_ context.Context, s *domain.MemorySuppress) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.suppress {
		if m.suppress[i].ID == s.ID {
			m.suppress[i] = *s
		}
	}
	return nil
}

func (m *memStore) GetSuppress(_ context.Context, id uuid.UUID) (*domain.MemorySuppress, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.suppress {
		if s.ID == id {
			return &s, nil
		}
	}
	return nil, nil
}

func (m *memStore) ListSuppress(context.Context) ([]domain.MemorySuppress, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]domain.MemorySuppress(nil), m.suppress...), nil
}

func (m *memStore) CreateEpisode(_ context.Context, e *domain.MemoryEpisode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ensureID(&e.UUIDModel)
	m.episodes = append([]domain.MemoryEpisode{*e}, m.episodes...)
	return nil
}

func (m *memStore) ListEpisodes(_ context.Context, limit int) ([]domain.MemoryEpisode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := append([]domain.MemoryEpisode(nil), m.episodes...)
	if len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

func (m *memStore) CreateDecision(_ context.Context, d *domain.Decision) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ensureID(&d.UUIDModel)
	m.decisions = append(m.decisions, *d)
	return nil
}

func (m *memStore) ClusterKeysSeenBefore(_ context.Context, runID uuid.UUID, keys []string) (map[string]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := map[string]bool{}
	for _, c := range m.clusters {
		if c.RunID != runID {
			for _, k := range keys {
				if c.Key == k {
					res[k] = true
				}
			}
		}
	}
	return res, nil
}
