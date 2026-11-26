package out

import "time"

// Viewer — кто смотрит сайт: GET /api/meta → viewer, GET /api/auth/me и ответы входа.
type Viewer struct {
	// Role — guest | buyer | ecom | product | content | head | system (system — Bearer или бэк-офис без входа).
	Role      string `json:"role"`
	RoleLabel string `json:"role_label"`
	// Name — имя вошедшего (демо-персона или логин); у гостя пусто.
	Name string `json:"name"`
	// ClientID — клиент витрины покупателя из сессии; у остальных null.
	ClientID   *string `json:"client_id"`
	ClientName *string `json:"client_name"`
	// Persona — демо-аккаунт; null — вход по паролю или гость.
	Persona *string `json:"persona"`
	// ExpiresAt — когда истекает сессия; null — сессии нет.
	ExpiresAt *time.Time `json:"expires_at"`
}

// AuthMe — GET /api/auth/me и ответ входа: {"viewer": …}.
type AuthMe struct {
	Viewer Viewer `json:"viewer"`
}

// AuthConfig — GET /api/auth/config: какие способы входа включены и демо-аккаунты для кнопок.
type AuthConfig struct {
	// DemoLogin — включены ли кнопки демо-аккаунтов (DEMO_LOGIN_BUTTONS).
	DemoLogin bool `json:"demo_login"`
	// AuthRequired — бэк-офис только после входа (AUTH_REQUIRED).
	AuthRequired bool `json:"auth_required"`
	// PasswordLogin — заданы ли аккаунты для входа по паролю (AUTH_ACCOUNTS).
	PasswordLogin bool `json:"password_login"`
	// Personas — демо-аккаунты; пусто, если DemoLogin выключен.
	Personas []Persona `json:"personas"`
}

// Persona — демо-аккаунт: вымышленный человек, роль и что он покажет.
type Persona struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Initials  string `json:"initials"`
	Role      string `json:"role"`
	RoleLabel string `json:"role_label"`
	// Title — должность или кто это для магазина.
	Title string `json:"title"`
	// Note — что покажет вход под этим аккаунтом.
	Note string `json:"note"`
	// ClientID, ClientName, ClientHint — клиент витрины покупателя (из справочника клиентов); у сотрудников null.
	ClientID   *string `json:"client_id"`
	ClientName *string `json:"client_name"`
	ClientHint *string `json:"client_hint"`
	// Pending — предложения агента, ждущие решения этой роли (у руководителя — все); null — очередь недоступна.
	Pending *int `json:"pending"`
}
