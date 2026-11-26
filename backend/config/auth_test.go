package config

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Вход по ролям: в development вход обязателен и демо-кнопки включены, в production демо-кнопки по умолчанию
// выключены, а выключить вход нельзя.
func TestAuthDefaults(t *testing.T) {
	clearEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.Auth.Required || !c.Auth.DemoLogin || c.Auth.SessionTTL != 12*time.Hour || c.Auth.LoginPerMin != 10 {
		t.Fatalf("development: %+v", c.Auth)
	}
	prod := func() {
		clearEnv(t)
		t.Setenv("APP_ENV", EnvProduction)
		t.Setenv("PUBLIC_ORIGIN", "https://shop.example.ru")
		t.Setenv("DATABASE_URL", "postgres://db")
		t.Setenv("TRUSTED_PROXIES", "172.16.0.0/12")
		t.Setenv("BACKOFFICE_TOKEN", testToken)
	}
	prod()
	if c, err = Load(); err != nil || !c.Auth.Required || c.Auth.DemoLogin {
		t.Fatalf("production: %+v %v", c.Auth, err)
	}
	prod()
	t.Setenv("DEMO_LOGIN_BUTTONS", "on")
	if c, err = Load(); err != nil || !c.Auth.DemoLogin {
		t.Fatalf("стенд для жюри: %+v %v", c.Auth, err)
	}
	prod()
	t.Setenv("AUTH_REQUIRED", "off")
	if _, err = Load(); err == nil || !strings.Contains(err.Error(), "AUTH_REQUIRED") {
		t.Fatalf("production без входа: %v", err)
	}
}

func TestAuthAccounts(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("x"), bcrypt.MinCost)
	h := string(hash)
	acc, err := parseAccounts("Olga:buyer:client-a:" + h + ", boss:head:" + h)
	if err != nil || len(acc) != 2 || acc[0].Login != "olga" || acc[0].ClientID != "client-a" || acc[1].Role != "head" {
		t.Fatalf("%+v %v", acc, err)
	}
	for name, raw := range map[string]string{
		"без хэша":          "olga:buyer:client-a",
		"роль":              "olga:admin:" + h,
		"покупатель без id": "olga:buyer:" + h,
		"id у сотрудника":   "olga:ecom:client-a:" + h,
		"не bcrypt":         "olga:head:plain",
		"повтор":            "olga:head:" + h + ",OLGA:ecom:" + h,
	} {
		if _, err := parseAccounts(raw); err == nil {
			t.Errorf("%s: ждали ошибку", name)
		} else if strings.Contains(err.Error(), h) {
			t.Errorf("%s: хэш в тексте ошибки", name)
		}
	}
	clearEnv(t)
	t.Setenv("AUTH_SESSION_TTL", "10s")
	if _, err := Load(); err == nil {
		t.Error("AUTH_SESSION_TTL короче минуты")
	}
}
