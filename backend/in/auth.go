package in

// Login — POST /api/auth/login: вход по логину и паролю (аккаунты AUTH_ACCOUNTS).
type Login struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// DemoLogin — POST /api/auth/demo-login и /api/auth/demo-switch: демо-аккаунт по id.
type DemoLogin struct {
	Persona string `json:"persona"`
}
