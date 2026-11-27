package handlers

import (
	"io"
	"net/http"

	"kuznica/api/pkg/errs"
	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
)

// Поиск по ТЗ (контур ТЗ, TZ-CORE §2): витрина, без входа; Origin, лимит частоты и срок запроса — общие для /api.
// Тело POST /api/tz — до 20 МиБ (server.go: MaxUploadBody, LimitBody); содержимое файла в лог не пишется.

// Тексты отказов загрузки.
const (
	MsgTZNoFile    = "Приложите файл технического задания"
	MsgTZOneFile   = "Загрузите один файл технического задания"
	MsgTZMultipart = "Файл ТЗ передаётся формой multipart/form-data"
)

// TZUpload — POST /api/tz: multipart, одно поле с файлом.
func (h *Handler) TZUpload(c *fiber.Ctx) error {
	form, err := c.MultipartForm()
	if err != nil {
		return errs.NewValidationError(MsgTZMultipart)
	}
	defer func() { _ = form.RemoveAll() }()
	var files int
	var name string
	var data []byte
	for _, fhs := range form.File {
		for _, fh := range fhs {
			files++
			if files > 1 {
				return errs.NewValidationError(MsgTZOneFile)
			}
			f, err := fh.Open()
			if err != nil {
				return errs.NewValidationError(MsgTZNoFile)
			}
			data, err = io.ReadAll(io.LimitReader(f, services.TZMaxFileBytes+1))
			_ = f.Close()
			if err != nil {
				return errs.NewValidationError(MsgTZNoFile)
			}
			name = fh.Filename
		}
	}
	if files == 0 {
		return errs.NewValidationError(MsgTZNoFile)
	}
	res, err := h.services.TZStart(services.WithClientIP(c.UserContext(), c.IP()), name, data)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// TZOperation — GET /api/tz/operations/{id}: статус и этап разбора.
func (h *Handler) TZOperation(c *fiber.Ctx) error {
	res, err := h.services.TZOperation(c.Params("id"))
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// TZCancel — DELETE /api/tz/operations/{id}: отмена разбора.
func (h *Handler) TZCancel(c *fiber.Ctx) error {
	if err := h.services.TZCancel(c.Params("id")); err != nil {
		return err
	}
	return c.SendStatus(http.StatusNoContent)
}

// TZResult — GET /api/tz/{hash}: позиции разобранного ТЗ; неизвестный или истёкший хеш — 404.
func (h *Handler) TZResult(c *fiber.Ctx) error {
	res, err := h.services.TZResult(c.UserContext(), c.Params("hash"))
	if err != nil {
		return err
	}
	return c.JSON(res)
}
