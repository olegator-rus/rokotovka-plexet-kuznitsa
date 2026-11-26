package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"kuznica/api/pkg/errs"

	"github.com/gofiber/fiber/v2"
)

// maxJSONBody — потолок JSON-тела запросов поиска и событий.
const maxJSONBody = 64 << 10

// bindBody — разбор JSON-тела в DTO слоя in: пустое тело, не JSON и второй документ после первого —
// отказ 422. Незнакомые поля допускаются: фронт и бэкенд меняются параллельно, новое поле контракта не
// должно ронять поиск. Значения проверяет сервис.
func bindBody(c *fiber.Ctx, dst any) error {
	body := c.Body()
	if len(body) > maxJSONBody {
		return fiber.ErrRequestEntityTooLarge
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(dst); err != nil {
		return errs.NewValidationError(errs.MsgInvalidJSON)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errs.NewValidationError(errs.MsgInvalidJSON)
	}
	return nil
}
