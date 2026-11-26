package services

import (
	"context"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Демо-аккаунты: вымышленные люди на mock-данных. Покупатели привязаны к клиентам из справочника
// (clients.json): нет такого клиента в загруженном каталоге — нет и кнопки. Сотрудники — по одному на роль.

type demoPersona struct {
	id, name, role, title, note, clientID string
}

var demoPersonas = []demoPersona{
	{id: "buyer-smb", name: "Ольга Северцева", role: RoleBuyer, title: "Закупки ИТ, малый бизнес", clientID: "client-a",
		note: "Покупала Kaspersky — на «продлить антивирус» первой будет её лицензия"},
	{id: "buyer-enterprise", name: "Артём Белозёров", role: RoleBuyer, title: "ИТ-директор, крупная компания", clientID: "client-c",
		note: "Windows Server, CAL, Veeam и KES в истории — своя выдача и допродажи к серверу"},
	{id: "buyer-gov", name: "Наталья Громова", role: RoleBuyer, title: "Контрактный управляющий, госсектор", clientID: "client-d",
		note: "Dr.Web и Secret Net в истории — продления этих лицензий первыми"},
	{id: "staff-ecom", name: "Илья Ветров", role: RoleEcom, title: "Менеджер e-com",
		note: "Аналитика: проблемные запросы; решает правки правил поиска и маржу"},
	{id: "staff-product", name: "Дарья Лапина", role: RoleProduct, title: "Продукт-менеджер",
		note: "Очередь агента: связи, комплекты, ассортимент; откат связей"},
	{id: "staff-content", name: "Елена Сорокина", role: RoleContent, title: "Контент-менеджер",
		note: "Карточки и термины; решения только по своим предложениям"},
	{id: "staff-head", name: "Сергей Орлов", role: RoleHead, title: "Руководитель e-com",
		note: "Видит всё, решает за любую роль, смотрит аналитику глазами команды"},
}

// DemoPersonas — демо-аккаунты для кнопок входа: покупатели только на клиентах загруженного каталога; у сотрудников —
// число ждущих решения предложений их роли (у руководителя — все), если очередь доступна.
func (s *Services) DemoPersonas(ctx context.Context) []out.Persona {
	clients := s.clientIndex()
	pending := s.pendingByRole(ctx)
	res := make([]out.Persona, 0, len(demoPersonas))
	for _, d := range demoPersonas {
		p := out.Persona{ID: d.id, Name: d.name, Initials: initials(d.name), Role: d.role, RoleLabel: RoleLabel(d.role),
			Title: d.title, Note: d.note}
		if d.clientID != "" {
			c, ok := clients[d.clientID]
			if !ok {
				continue
			}
			id, name, hint := c.ID, c.Name, c.Hint
			p.ClientID, p.ClientName, p.ClientHint = &id, &name, &hint
		}
		if pending != nil && IsStaffRole(d.role) {
			n := 0
			for role, k := range pending {
				if d.role == RoleHead || role == d.role {
					n += k
				}
			}
			p.Pending = &n
		}
		res = append(res, p)
	}
	return res
}

// DemoPersona — демо-аккаунт по id (с клиентом из каталога); false — такого нет.
func (s *Services) DemoPersona(id string) (out.Persona, bool) {
	clients := s.clientIndex()
	for _, d := range demoPersonas {
		if d.id != id {
			continue
		}
		p := out.Persona{ID: d.id, Name: d.name, Initials: initials(d.name), Role: d.role, RoleLabel: RoleLabel(d.role),
			Title: d.title, Note: d.note}
		if d.clientID != "" {
			c, ok := clients[d.clientID]
			if !ok {
				return out.Persona{}, false
			}
			cid, name := c.ID, c.Name
			p.ClientID, p.ClientName = &cid, &name
		}
		return p, true
	}
	return out.Persona{}, false
}

// ClientName — имя клиента витрины из справочника; "" — нет такого.
func (s *Services) ClientName(id string) string {
	return s.clientIndex()[id].Name
}

// clientIndex — клиенты загруженного каталога по id (как в /api/meta).
func (s *Services) clientIndex() map[string]out.MetaClient {
	idx := map[string]out.MetaClient{}
	st := s.catalog.Load()
	if st == nil {
		return idx
	}
	for _, c := range st.clients {
		idx[c.ID] = out.MetaClient{ID: c.ID, Name: c.Name, Segment: c.Segment, Hint: clientHint(st, c)}
	}
	return idx
}

// pendingByRole — ждущие решения предложения по ролям; nil — очередь недоступна (нет базы).
func (s *Services) pendingByRole(ctx context.Context) map[string]int {
	st := s.store()
	if st == nil {
		return nil
	}
	list, err := s.Proposals(ctx, domain.ProposalPending, "")
	if err != nil {
		return nil
	}
	res := map[string]int{}
	for _, p := range list.Items {
		res[p.Role]++
	}
	return res
}

// initials — «Ольга Северцева» → «ОС».
func initials(name string) string {
	var b strings.Builder
	for _, w := range strings.Fields(name) {
		r := []rune(w)
		b.WriteRune(r[0])
		if b.Len() >= 4 {
			break
		}
	}
	return strings.ToUpper(b.String())
}
