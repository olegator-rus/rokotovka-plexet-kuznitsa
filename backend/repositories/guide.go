package repositories

import (
	"context"

	"kuznica/api/domain"
)

// SaveGuideTurn — журнал хода подбора (контур подбора).
func (r *Repos) SaveGuideTurn(ctx context.Context, t *domain.GuideTurn) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Create(t).Error
}
