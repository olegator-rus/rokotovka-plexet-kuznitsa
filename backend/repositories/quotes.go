package repositories

import (
	"context"

	"kuznica/api/domain"
)

// Запросы КП.

// CreateQuote — новый запрос КП.
func (r *Repos) CreateQuote(ctx context.Context, q *domain.QuoteRequest) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Create(q).Error
}

// ListClientQuotes — последние запросы КП клиента витрины, новые первыми (вход покупателя).
func (r *Repos) ListClientQuotes(ctx context.Context, clientID string, limit int) ([]domain.QuoteRequest, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []domain.QuoteRequest
	err := r.db.WithContext(ctx).Where("client_id = ?", clientID).Order("created_at desc").Limit(limit).Find(&rows).Error
	return rows, err
}

// ListQuotes — последние запросы КП, новые первыми.
func (r *Repos) ListQuotes(ctx context.Context, limit int) ([]domain.QuoteRequest, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []domain.QuoteRequest
	err := r.db.WithContext(ctx).Order("created_at desc").Limit(limit).Find(&rows).Error
	return rows, err
}
