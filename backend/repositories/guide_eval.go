package repositories

import (
	"context"

	"kuznica/api/domain"
)

// GuideTurnsOf — журнал ходов подборов selectionIDs (оценка), по порядку хода.
func (r *Repos) GuideTurnsOf(ctx context.Context, selectionIDs []string) ([]domain.GuideTurn, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []domain.GuideTurn
	if len(selectionIDs) == 0 {
		return rows, nil
	}
	err := r.db.WithContext(ctx).Where("selection_id IN ?", selectionIDs).Order("selection_id, turn, turn_started_at").Find(&rows).Error
	return rows, err
}
