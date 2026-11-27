package services

import (
	"context"
	"errors"
	"strings"

	"kuznica/api/out"
	"kuznica/api/pkg/errs"
	"kuznica/api/repositories"
)

// Трасса сохранённого поиска (переход из журнала «Экономики» и из аналитики в разбор запроса).

// MsgTraceNotFound — трассы с таким request_id нет.
const MsgTraceNotFound = "Трассировка поиска не найдена: поиск не записывался в журнал или удалён"

// Trace — GET /api/traces/{request_id}.
func (s *Services) Trace(ctx context.Context, requestID string) (*out.TraceView, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || len(requestID) > 32 {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(
			errs.FieldError{Field: "request_id", Code: errs.CodeInvalidFormat, Message: "Номер поиска — до 32 символов"})
	}
	if !s.repos.Enabled() {
		return nil, errs.NewUnavailableError("Журнал поиска недоступен: сервис запущен без базы данных")
	}
	row, err := s.repos.TraceByRequestID(ctx, requestID)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, errs.NewNotFoundError(MsgTraceNotFound)
	}
	if err != nil {
		return nil, err
	}
	return &out.TraceView{
		RequestID: row.RequestID, Query: row.Query, Normalized: row.Normalized, Config: row.Config, Route: row.Route,
		Source: row.Source, ClientID: row.ClientID, CreatedAt: row.CreatedAt, Trace: row.Trace,
	}, nil
}
