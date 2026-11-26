package config

import (
	"strings"
	"testing"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"APP_ENV", "PORT", "PUBLIC_ORIGIN", "DATABASE_URL", "TRUSTED_PROXIES"} {
		t.Setenv(k, "")
	}
}

func TestLoadDevelopmentDefaults(t *testing.T) {
	clearEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != defaultAddr || c.PublicOrigin != devOrigin || c.IsProduction() {
		t.Fatalf("неожиданные значения по умолчанию: %+v", c)
	}
}

func TestLoadPortForms(t *testing.T) {
	for in, want := range map[string]string{"9000": "127.0.0.1:9000", "0.0.0.0:8080": "0.0.0.0:8080", ":8080": ":8080"} {
		clearEnv(t)
		t.Setenv("PORT", in)
		c, err := Load()
		if err != nil || c.Addr != want {
			t.Fatalf("PORT=%q → %q, err %v; ожидалось %q", in, c.Addr, err, want)
		}
	}
}

func TestLoadNormalizesOrigin(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_ENV", EnvProduction)
	t.Setenv("PUBLIC_ORIGIN", "https://shop.example.ru/")
	t.Setenv("DATABASE_URL", "postgres://db")
	t.Setenv("TRUSTED_PROXIES", "172.16.0.0/12")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.PublicOrigin != "https://shop.example.ru" {
		t.Fatalf("PublicOrigin = %q", c.PublicOrigin)
	}
}

func TestLoadRejectsBadOrigin(t *testing.T) {
	for _, v := range []string{"*", "shop.example.ru", "https://shop.example.ru/app", "ftp://x"} {
		clearEnv(t)
		t.Setenv("PUBLIC_ORIGIN", v)
		if _, err := Load(); err == nil {
			t.Fatalf("PUBLIC_ORIGIN=%q должен отклоняться", v)
		}
	}
}

// В production без origin, базы и доверенного прокси API не стартует; все причины — одной сводкой.
func TestLoadProductionRequirements(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_ENV", EnvProduction)
	_, err := Load()
	for _, want := range []string{"PUBLIC_ORIGIN", "DATABASE_URL", "TRUSTED_PROXIES"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("ожидался отказ с %s, получено: %v", want, err)
		}
	}
}

func TestLoadRejectsUnknownEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_ENV", "staging")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "APP_ENV") {
		t.Fatalf("APP_ENV=staging должен отклоняться, получено: %v", err)
	}
}

func TestLoadTrustedProxies(t *testing.T) {
	clearEnv(t)
	t.Setenv("TRUSTED_PROXIES", " 10.0.0.1 , 172.16.0.0/12,")
	c, err := Load()
	if err != nil || len(c.TrustedProxies) != 2 || c.TrustedProxies[1] != "172.16.0.0/12" {
		t.Fatalf("TrustedProxies = %v, err %v", c.TrustedProxies, err)
	}
	clearEnv(t)
	t.Setenv("TRUSTED_PROXIES", "caddy")
	if _, err := Load(); err == nil {
		t.Fatal("имя хоста вместо IP должно отклоняться")
	}
}

func TestSummaryHidesSecrets(t *testing.T) {
	c := Config{AppEnv: EnvDevelopment, Addr: defaultAddr, DatabaseURL: "postgres://u:secret@h/db"}
	if s := strings.Join(c.Summary(), "\n"); strings.Contains(s, "secret") {
		t.Fatalf("Summary выдаёт секрет: %s", s)
	}
}
