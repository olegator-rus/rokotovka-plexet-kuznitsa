package services

import (
	"context"
	"fmt"
	"unicode"
)

// Роли и права. Сервер проверяет права на каждое решение; фронт только прячет и подсказывает.
//
//	guest   — не вошёл: витрина, «До / После», правила рекомендаций, запрос КП;
//	buyer   — покупатель-компания: то же, выдача по своему клиенту из сессии;
//	ecom    — администратор поиска: весь бэк-офис; решает причины «поиск не понял», «стоит низко», «не о товаре»,
//	          откат правил поиска, политика ранжирования (рычаг маржи);
//	product — категорийный и лицензионный менеджер: весь бэк-офис; причины «нет в каталоге», «ошибка в связи или
//	          правиле», откат справочника связей;
//	content — каталог: весь бэк-офис; причина «в карточке не хватает данных»;
//
// Владелец предложения — владелец его причины (causes.go, контур доводки), а не роль, записанная агентом.
//
//	head    — руководитель e-com: всё;
//	system  — Bearer BACKOFFICE_TOKEN (скрипты и CLI) или бэк-офис без входа (AUTH_REQUIRED=off в development): всё.
const (
	RoleGuest   = "guest"
	RoleBuyer   = "buyer"
	RoleEcom    = "ecom"
	RoleProduct = "product"
	RoleContent = "content"
	RoleHead    = "head"
	RoleSystem  = "system"
)

// roleLabels — роли по-русски (подписи в интерфейсе и в отказах).
var roleLabels = map[string]string{
	RoleGuest:   "Гость",
	RoleBuyer:   "Покупатель",
	RoleEcom:    "Администратор поиска",
	RoleProduct: "Категорийный и лицензионный менеджер",
	RoleContent: "Каталог",
	RoleHead:    "Руководитель e-com",
	RoleSystem:  "Служебный доступ",
}

// RoleLabel — роль по-русски; неизвестная — как есть.
func RoleLabel(role string) string {
	if l, ok := roleLabels[role]; ok {
		return l
	}
	return role
}

// IsStaffRole — сотрудник магазина (вход в бэк-офис): четыре роли и служебный доступ.
func IsStaffRole(role string) bool {
	switch role {
	case RoleEcom, RoleProduct, RoleContent, RoleHead, RoleSystem:
		return true
	}
	return false
}

// Области решений вне очереди предложений: чья это зона.
const (
	// AreaRules — откат правил поиска (POST /api/configs/rollback).
	AreaRules = RoleEcom
	// AreaRelations — откат справочника связей (POST /api/relations/rollback).
	AreaRelations = RoleProduct
	// AreaMargin — применение и откат политики маржи (POST /api/margin/policy, /rollback).
	AreaMargin = RoleEcom
)

// CanDecide — может ли actor принять решение в зоне роли owner (role предложения или область выше).
// Руководитель и служебный доступ решают всё; остальные — только своё.
func CanDecide(actor, owner string) bool {
	switch actor {
	case RoleHead, RoleSystem:
		return true
	case RoleEcom, RoleProduct, RoleContent:
		return actor == owner
	}
	return false
}

// DenyMessage — отказ по праву: «Это решение принимает <роль>».
func DenyMessage(owner string) string {
	l := RoleLabel(owner)
	if l == owner || owner == "" {
		return "Это решение принимает руководитель e-com"
	}
	if owner == RoleContent {
		l = "команда каталога"
	}
	return fmt.Sprintf("Это решение принимает %s", lowerFirst(l))
}

// lowerFirst — «Администратор поиска» → «администратор поиска».
func lowerFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

// ProposalRole — кто по таблице прав принимает решение по предложению: владелец его причины.
func (s *Services) ProposalRole(ctx context.Context, id string) (string, error) {
	p, err := s.loadProposal(ctx, id)
	if err != nil {
		return "", err
	}
	return OwnerOf(p.Diagnosis), nil
}
