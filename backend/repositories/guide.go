package repositories

import (
	"context"
	"time"

	"kuznica/api/domain"
)

// SaveGuideTurn — журнал хода подбора (контур подбора).
func (r *Repos) SaveGuideTurn(ctx context.Context, t *domain.GuideTurn) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Create(t).Error
}

// GuideEvalSessionPrefix — сессии прогона оценки подбора (`go run . guide-eval`): в отчёт SearchOps не попадают,
// иначе вопросы набора оценки («найди корм для кота», «ывапролдж») считаются спросом покупателей, по разу на прогон.
const GuideEvalSessionPrefix = "gev"

// GuideGapItems — пробелы подбора (guide_turns.gap_items) начиная с since, новые первыми; до limit ходов. Ходы прогона
// оценки исключены.
func (r *Repos) GuideGapItems(ctx context.Context, since time.Time, limit int) ([]domain.JSONB, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []domain.JSONB
	err := r.db.WithContext(ctx).Model(&domain.GuideTurn{}).
		Where("created_at >= ? AND gap_items IS NOT NULL AND gap_items::text <> '[]'", since).
		Where("session_id IS NULL OR session_id NOT LIKE ?", GuideEvalSessionPrefix+"%").
		Order("created_at DESC").Limit(limit).Pluck("gap_items", &rows).Error
	return rows, err
}
