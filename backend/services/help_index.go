package services

import (
	"context"
	"errors"
	"strings"
	"sync"

	"go.uber.org/zap"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/out"
)

// Справка магазина: небольшой индекс статей — счёт, документы, доставка, возврат,
// продление, активация. Сервисный запрос получает ответ и ссылку без товаров; смешанный («продлить касперский»)
// — товар сверху и строку «Как продлить в кабинете». Правила, без модели и без движка.

// Виды ответа справки.
const (
	HelpModeAnswer = "answer"
	HelpModeHint   = "hint"
)

// procurementCache — файлы контура доводки из источника каталога, читаются один раз.
type procurementCache struct {
	once   sync.Once
	help   []domain.HelpArticle
	reestr map[string]domain.ReestrEntry
}

type helpLoader interface {
	LoadHelp(ctx context.Context) ([]domain.HelpArticle, error)
}

type reestrLoader interface {
	LoadReestr(ctx context.Context) ([]domain.ReestrEntry, error)
}

// procurementData — справка и реестр; источник без файлов (generic) — пусто: справки нет, статус реестра по
// происхождению.
func (s *Services) procurementData() *procurementCache {
	c := &s.procurement
	c.once.Do(func() {
		c.reestr = map[string]domain.ReestrEntry{}
		ctx := context.Background()
		if l, ok := s.source.(helpLoader); ok {
			hs, err := l.LoadHelp(ctx)
			if err == nil {
				c.help = hs
			} else if !errors.Is(err, catalogsource.ErrNoFile) {
				zap.L().Warn("справка: файл не прочитан", zap.Error(err))
			}
		}
		if l, ok := s.source.(reestrLoader); ok {
			rs, err := l.LoadReestr(ctx)
			if err == nil {
				for _, r := range rs {
					c.reestr[r.FamilyID] = r
				}
			} else if !errors.Is(err, catalogsource.ErrNoFile) {
				zap.L().Warn("реестр: файл не прочитан", zap.Error(err))
			}
		}
	})
	return c
}

// matchHelp — статья, чей триггер (слово или фраза) встречается в запросе; при нескольких — самый длинный
// триггер. nil — запрос не сервисный.
func matchHelp(articles []domain.HelpArticle, tokens []string) *domain.HelpArticle {
	q := " " + strings.Join(tokens, " ") + " "
	var best *domain.HelpArticle
	bestLen := 0
	for i := range articles {
		for _, t := range articles[i].Triggers {
			ft := strings.Join(wordsOf(t), " ")
			if ft == "" || !strings.Contains(q, " "+ft+" ") {
				continue
			}
			if n := len([]rune(ft)); n > bestLen {
				best, bestLen = &articles[i], n
			}
		}
	}
	return best
}

func helpAnswer(a *domain.HelpArticle, mode string) *out.HelpAnswer {
	return &out.HelpAnswer{ID: a.ID, Mode: mode, Title: a.Title, Text: a.Text, LinkLabel: a.LinkLabel, URL: a.URL}
}

// helpFor — ответ справки для запроса: answer — запрос сервисный (вопрос не о товаре или триггер справки без
// единого названного товара, вендора и категории), hint — продление названного продукта или категории.
func (s *Services) helpFor(tokens []string, attrs queryAttrs, ent queryEntities) *out.HelpAnswer {
	arts := s.procurementData().help
	if len(arts) == 0 {
		return nil
	}
	named := len(ent.vendors) > 0 || len(ent.families) > 0 || ent.parsedFamily != ""
	if a := matchHelp(arts, tokens); a != nil && (ent.nonProduct || !named && ent.category == "" && !attrs.analog) {
		return helpAnswer(a, HelpModeAnswer)
	}
	if attrs.renewal && (named || ent.category != "") {
		for i := range arts {
			if arts[i].Mixed == DealRenewal {
				return helpAnswer(&arts[i], HelpModeHint)
			}
		}
	}
	return nil
}

// HelpArticles — GET /api/help (витрина): статьи справки для страницы помощи.
func (s *Services) HelpArticles() []out.HelpAnswer {
	arts := s.procurementData().help
	res := make([]out.HelpAnswer, 0, len(arts))
	for i := range arts {
		res = append(res, *helpAnswer(&arts[i], HelpModeAnswer))
	}
	return res
}

// applyHelpAnswer — сервисный запрос: ответ справки вместо товаров (выдача, блоки допродажи и фасеты пустые).
func applyHelpAnswer(resp *out.SearchResponse) {
	resp.Items = []out.SearchItem{}
	resp.Total = 0
	resp.Complements = []out.Complement{}
	resp.Companions, resp.Upgrade = nil, nil
	resp.Facets = []out.Facet{}
	resp.Refinements = nil
	resp.Relaxed = []out.Relaxed{}
	resp.RelatedQueries = []string{}
	resp.Clarify = nil
	if resp.Counts != nil {
		resp.Counts = &out.ResultCounts{}
	}
}
