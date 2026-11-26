package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// AuthConfig — вход на сайт по ролям: кто может войти, нужен ли вход в бэк-офис, демо-аккаунты.
type AuthConfig struct {
	// Required — AUTH_REQUIRED: бэк-офис только после входа сотрудника (или с Bearer BACKOFFICE_TOKEN). По
	// умолчанию on. off — прежний режим: без BACKOFFICE_TOKEN бэк-офис открыт (только development).
	Required bool
	// DemoLogin — DEMO_LOGIN_BUTTONS: вход в демо-аккаунты одной кнопкой (POST /api/auth/demo-login). По умолчанию
	// on в development и off в production; на стенде для жюри включается явно.
	DemoLogin bool
	// Accounts — AUTH_ACCOUNTS: аккаунты для входа по логину и паролю через запятую, каждый
	// login:role[:client_id]:bcrypt-хэш (role — buyer|ecom|product|content|head, client_id — только у buyer).
	// Пусто — только демо-вход и Bearer.
	Accounts []Account
	// SessionTTL — AUTH_SESSION_TTL: срок сессии (Go duration), по умолчанию 12h.
	SessionTTL time.Duration
	// LoginPerMin — AUTH_LOGIN_PER_MIN: неудачных попыток входа в минуту с адреса, по умолчанию 10; 0 — без лимита.
	LoginPerMin int
}

// Account — аккаунт из AUTH_ACCOUNTS. Hash — bcrypt-хэш пароля; сам пароль сервер не знает.
type Account struct {
	Login    string
	Role     string
	ClientID string
	Hash     string
}

// AccountRoles — роли, которые можно выдать аккаунту.
var AccountRoles = []string{"buyer", "ecom", "product", "content", "head"}

const (
	defaultSessionTTL  = 12 * time.Hour
	defaultLoginPerMin = 10
)

// loadAuth — настройки входа. dev — APP_ENV=development.
func loadAuth(dev bool) (AuthConfig, error) {
	a := AuthConfig{Required: true, DemoLogin: dev, SessionTTL: defaultSessionTTL, LoginPerMin: defaultLoginPerMin}
	var problems []string
	flag := func(key string, dst *bool) {
		switch v := strings.ToLower(env(key, "")); v {
		case "":
		case "on", "true", "1":
			*dst = true
		case "off", "false", "0":
			*dst = false
		default:
			problems = append(problems, fmt.Sprintf("%s=%q: ожидается on или off", key, v))
		}
	}
	flag("AUTH_REQUIRED", &a.Required)
	flag("DEMO_LOGIN_BUTTONS", &a.DemoLogin)
	if v := env("AUTH_SESSION_TTL", ""); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < time.Minute || d > 30*24*time.Hour {
			problems = append(problems, fmt.Sprintf("AUTH_SESSION_TTL=%q: ожидается срок от 1m до 720h", v))
		} else {
			a.SessionTTL = d
		}
	}
	if v := env("AUTH_LOGIN_PER_MIN", ""); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			problems = append(problems, fmt.Sprintf("AUTH_LOGIN_PER_MIN=%q: ожидается неотрицательное число", v))
		} else {
			a.LoginPerMin = n
		}
	}
	accounts, err := parseAccounts(env("AUTH_ACCOUNTS", ""))
	if err != nil {
		problems = append(problems, err.Error())
	}
	a.Accounts = accounts
	if !a.Required && !dev {
		problems = append(problems, "AUTH_REQUIRED=off: в production бэк-офис без входа не запускается")
	}
	if len(problems) > 0 {
		return a, fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return a, nil
}

// parseAccounts — AUTH_ACCOUNTS: «login:role[:client_id]:hash» через запятую. В хэше bcrypt двоеточий и запятых нет.
func parseAccounts(raw string) ([]Account, error) {
	var out []Account
	var problems []string
	seen := map[string]bool{}
	for i, item := range list(raw) {
		parts := strings.Split(item, ":")
		var acc Account
		switch len(parts) {
		case 3:
			acc = Account{Login: parts[0], Role: parts[1], Hash: parts[2]}
		case 4:
			acc = Account{Login: parts[0], Role: parts[1], ClientID: parts[2], Hash: parts[3]}
		default:
			problems = append(problems, fmt.Sprintf("AUTH_ACCOUNTS[%d]: ожидается login:role[:client_id]:bcrypt-хэш", i))
			continue
		}
		acc.Login = strings.ToLower(strings.TrimSpace(acc.Login))
		acc.Role = strings.ToLower(strings.TrimSpace(acc.Role))
		acc.ClientID = strings.TrimSpace(acc.ClientID)
		where := fmt.Sprintf("AUTH_ACCOUNTS[%d] (%s)", i, acc.Login)
		switch {
		case acc.Login == "" || len(acc.Login) > 64:
			problems = append(problems, fmt.Sprintf("AUTH_ACCOUNTS[%d]: пустой или слишком длинный логин", i))
		case seen[acc.Login]:
			problems = append(problems, where+": логин повторяется")
		case !validRole(acc.Role):
			problems = append(problems, where+": роль — buyer, ecom, product, content или head")
		case acc.Role == "buyer" && acc.ClientID == "":
			problems = append(problems, where+": покупателю нужен client_id")
		case acc.Role != "buyer" && acc.ClientID != "":
			problems = append(problems, where+": client_id бывает только у buyer")
		default:
			if _, err := bcrypt.Cost([]byte(acc.Hash)); err != nil {
				problems = append(problems, where+": пароль — bcrypt-хэш ($2a$/$2b$…)")
				continue
			}
			seen[acc.Login] = true
			out = append(out, acc)
		}
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return out, nil
}

func validRole(r string) bool {
	for _, x := range AccountRoles {
		if x == r {
			return true
		}
	}
	return false
}

// summary — строка сводки входа без логинов и хэшей.
func (a AuthConfig) summary() string {
	onOff := func(b bool) string {
		if b {
			return "on"
		}
		return "off"
	}
	return fmt.Sprintf("вход: обязателен для бэк-офиса %s, демо-аккаунты %s, аккаунтов по паролю %d, сессия %s, попыток входа %d/мин",
		onOff(a.Required), onOff(a.DemoLogin), len(a.Accounts), a.SessionTTL, a.LoginPerMin)
}
