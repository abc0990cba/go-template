package repository

import (
	"context"
	"sort"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/settings"
)

// UpsertSetting stores one setting.
func (s *MemoryStore) UpsertSetting(_ context.Context, setting settings.Setting) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings[setting.Key] = setting
	return nil
}

// GetSetting returns one setting by key.
func (s *MemoryStore) GetSetting(_ context.Context, key string) (settings.Setting, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	setting, ok := s.settings[key]
	if !ok {
		return settings.Setting{}, kernel.ErrNotFound
	}
	return setting, nil
}

// ListSettings returns every setting ordered by key.
func (s *MemoryStore) ListSettings(_ context.Context) ([]settings.Setting, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]settings.Setting, 0, len(s.settings))
	for _, setting := range s.settings {
		out = append(out, setting)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}
