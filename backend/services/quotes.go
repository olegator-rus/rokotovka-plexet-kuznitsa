package services

import (
	"context"
	"fmt"
	"math"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
)

// Спецификация и запрос КП. Цены — из каталога сервера, не от покупателя:
// сумма только если известны все цены, иначе null и число позиций «по запросу».

const (
	maxQuoteItems    = 50
	maxQuoteQuantity = 100000
	maxQuoteComment  = 2000
	quoteListLimit   = 200
)

// quoteOrigins — откуда позиция попала в спецификацию.
var quoteOrigins = map[string]bool{"search": true, "companion": true, "template": true, "guide": true}

// priceKnown — цена позиции точная: фиксированная и указана. «от …», по запросу, индивидуальная, нет в
// наличии — неизвестна.
func priceKnown(p *domain.Product) bool {
	mode := p.PriceMode
	if mode == "" {
		mode = domain.PriceFixed
	}
	return p.PriceRub != nil && !p.PriceOnRequest && mode == domain.PriceFixed
}

// quoteTotals — позиции, сумма (nil — есть цены по запросу) и число позиций без цены.
func quoteTotals(products map[string]*domain.Product, items []domain.QuoteItem) (*float64, int) {
	total, onRequest := 0.0, 0
	for _, it := range items {
		p := products[it.ProductID]
		if p == nil || !priceKnown(p) {
			onRequest++
			continue
		}
		total += *p.PriceRub * float64(it.Quantity)
	}
	if onRequest > 0 {
		return nil, onRequest
	}
	total = math.Round(total*100) / 100
	return &total, 0
}

// CreateQuote — POST /api/quotes: проверка спецификации по каталогу, запись, итог.
func (s *Services) CreateQuote(ctx context.Context, req in.QuoteRequest) (*out.QuoteCreated, error) {
	st := s.catalog.Load()
	var products map[string]*domain.Product
	if st != nil && st.commerce != nil {
		products = st.commerce.products
	}
	var bad []errs.FieldError
	switch {
	case len(req.Items) == 0:
		bad = append(bad, errs.FieldError{Field: "items", Code: errs.CodeRequired, Message: "Спецификация пуста"})
	case len(req.Items) > maxQuoteItems:
		bad = append(bad, errs.FieldError{Field: "items", Code: errs.CodeOutOfRange, Message: fmt.Sprintf("Позиций больше %d", maxQuoteItems)})
	}
	if len(req.SessionID) > maxIDLen || len(req.ClientID) > maxIDLen {
		bad = append(bad, errs.FieldError{Field: "session_id", Code: errs.CodeOutOfRange, Message: "Слишком длинный идентификатор"})
	}
	if len([]rune(req.Comment)) > maxQuoteComment {
		bad = append(bad, errs.FieldError{Field: "comment", Code: errs.CodeOutOfRange, Message: fmt.Sprintf("Комментарий длиннее %d символов", maxQuoteComment)})
	}
	items := make([]domain.QuoteItem, 0, len(req.Items))
	seen := map[string]bool{}
	for i, it := range req.Items {
		field := fmt.Sprintf("items[%d]", i)
		switch {
		case it.ProductID == "" || len(it.ProductID) > maxIDLen:
			bad = append(bad, errs.FieldError{Field: field + ".product_id", Code: errs.CodeRequired, Message: "Нужен id позиции"})
		case products != nil && products[it.ProductID] == nil:
			bad = append(bad, errs.FieldError{Field: field + ".product_id", Code: errs.CodeInvalidFormat, Message: "Позиции нет в каталоге"})
		case seen[it.ProductID+"|"+it.Origin]:
			bad = append(bad, errs.FieldError{Field: field + ".product_id", Code: errs.CodeInvalidFormat, Message: "Позиция повторяется"})
		case it.Quantity < 1 || it.Quantity > maxQuoteQuantity:
			bad = append(bad, errs.FieldError{Field: field + ".quantity", Code: errs.CodeOutOfRange, Message: fmt.Sprintf("Количество от 1 до %d", maxQuoteQuantity)})
		case !quoteOrigins[it.Origin]:
			bad = append(bad, errs.FieldError{Field: field + ".origin", Code: errs.CodeInvalidFormat, Message: "Происхождение — search, companion, template или guide"})
		case it.Origin == "companion" && len(it.CompanionClass) > 32:
			bad = append(bad, errs.FieldError{Field: field + ".companion_class", Code: errs.CodeOutOfRange, Message: "Неизвестный класс"})
		}
		seen[it.ProductID+"|"+it.Origin] = true
		cls := ""
		if it.Origin == "companion" {
			cls = it.CompanionClass
		}
		purchase, pbad := quotePurchase(field, it.Purchase)
		bad = append(bad, pbad...)
		items = append(items, domain.QuoteItem{ProductID: it.ProductID, Quantity: it.Quantity, Origin: it.Origin,
			CompanionClass: cls, Purchase: purchase})
	}
	if len(bad) > 0 {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	if products == nil {
		return nil, errs.NewUnavailableError("Каталог не загружен: цены проверить нельзя")
	}
	if !s.repos.Enabled() {
		return nil, errs.NewUnavailableError("Сервис запущен без базы данных: запрос КП не записан")
	}
	total, onRequest := quoteTotals(products, items)
	q := &domain.QuoteRequest{SessionID: optional(req.SessionID), ClientID: optional(req.ClientID), Items: items,
		Comment: strings.TrimSpace(req.Comment), Status: domain.QuoteNew, Positions: len(items), TotalRub: total,
		OnRequest: onRequest}
	if err := s.repos.CreateQuote(ctx, q); err != nil {
		return nil, err
	}
	return &out.QuoteCreated{ID: q.ID.String(), Status: q.Status, Positions: q.Positions, TotalRub: q.TotalRub,
		OnRequest: q.OnRequest}, nil
}

// Quotes — GET /api/quotes (бэк-офис): последние запросы КП с названиями и ценами позиций по текущему каталогу.
func (s *Services) Quotes(ctx context.Context) (*out.QuoteList, error) {
	if !s.repos.Enabled() {
		return &out.QuoteList{Items: []out.Quote{}}, nil
	}
	rows, err := s.repos.ListQuotes(ctx, quoteListLimit)
	if err != nil {
		return nil, err
	}
	return s.quoteList(rows), nil
}

// ClientQuotes — GET /api/quotes/mine: запросы КП покупателя (клиент — из сессии входа).
func (s *Services) ClientQuotes(ctx context.Context, clientID string) (*out.QuoteList, error) {
	if !s.repos.Enabled() || clientID == "" {
		return &out.QuoteList{Items: []out.Quote{}}, nil
	}
	rows, err := s.repos.ListClientQuotes(ctx, clientID, quoteListLimit)
	if err != nil {
		return nil, err
	}
	return s.quoteList(rows), nil
}

// quoteList — строки запросов КП с названиями и ценами позиций по текущему каталогу.
func (s *Services) quoteList(rows []domain.QuoteRequest) *out.QuoteList {
	var products map[string]*domain.Product
	if st := s.catalog.Load(); st != nil && st.commerce != nil {
		products = st.commerce.products
	}
	res := &out.QuoteList{Items: make([]out.Quote, 0, len(rows))}
	for _, q := range rows {
		item := out.Quote{QuoteCreated: out.QuoteCreated{ID: q.ID.String(), Status: q.Status, Positions: q.Positions,
			TotalRub: q.TotalRub, OnRequest: q.OnRequest}, CreatedAt: q.CreatedAt, SessionID: q.SessionID,
			ClientID: q.ClientID, Comment: q.Comment, Items: make([]out.QuoteLine, 0, len(q.Items))}
		for _, it := range q.Items {
			line := out.QuoteLine{ProductID: it.ProductID, Quantity: it.Quantity, Origin: it.Origin,
				CompanionClass: it.CompanionClass, PriceMode: domain.PriceQuoteRequired, Purchase: it.Purchase}
			if p := products[it.ProductID]; p != nil {
				line.Title, line.PriceMode = p.Title, p.PriceMode
				if priceKnown(p) {
					line.PriceRub = p.PriceRub
				}
			}
			item.Items = append(item.Items, line)
		}
		res.Items = append(res.Items, item)
	}
	return res
}
