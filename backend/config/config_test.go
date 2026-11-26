package config

import (
	"os"
	"strings"
	"testing"
)

// envKeys — все переменные, которые читает пакет.
var envKeys = []string{"APP_ENV", "PORT", "PUBLIC_ORIGIN", "DATABASE_URL", "TRUSTED_PROXIES", "OPENSEARCH_URL",
	"EMBEDDER_URL", "RERANKER_URL", "DATA_DIR", "DATA_SOURCE", "DATA_MAPPING", "INDEX_ALIAS", "LLM_PROVIDER", "LLM_BASE_URL", "LLM_MODEL", "LLM_FOLDER_ID",
	"LLM_API_KEY", "LLM_MODE", "LLM_INTENT_MODEL", "LLM_FALLBACK_MODEL", "LLM_JUDGE_MODEL",
	"ADMIN_ENABLED", "RATE_LIMIT_PER_MIN", "LLM_DAILY_BUDGET_RUB", "AGENT_BUDGET_RESERVE_RUB", "LLM_PER_IP_DAILY_RUB",
	"BACKOFFICE_TOKEN"}

// testToken — токен бэк-офиса для production-конфигураций тестов.
const testToken = "test-backoffice-token-0123456789"

// clearEnv — окружение без наших переменных: t.Setenv запоминает значение для восстановления, Unsetenv
// убирает переменную (для адресов сервисов «не задана» и «пустая» означают разное).
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range envKeys {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
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
	t.Setenv("BACKOFFICE_TOKEN", testToken)
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
	for _, want := range []string{"PUBLIC_ORIGIN", "DATABASE_URL", "TRUSTED_PROXIES", "BACKOFFICE_TOKEN"} {
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

// В development адреса сервисов по умолчанию локальные; пустое значение или off выключает шаг.
func TestServiceURLs(t *testing.T) {
	clearEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.OpenSearchURL != devOpenSearchURL || c.EmbedderURL != devModelsURL || c.RerankerURL != devModelsURL || c.DataDir != defaultDataDir {
		t.Fatalf("значения по умолчанию: %+v", c)
	}
	clearEnv(t)
	t.Setenv("EMBEDDER_URL", "")
	t.Setenv("RERANKER_URL", "off")
	t.Setenv("OPENSEARCH_URL", "http://os:9200/")
	c, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.EmbedderURL != "" || c.RerankerURL != "" || c.OpenSearchURL != "http://os:9200" {
		t.Fatalf("выключение и нормализация адресов: %+v", c)
	}
	clearEnv(t)
	t.Setenv("OPENSEARCH_URL", "os:9200")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "OPENSEARCH_URL") {
		t.Fatalf("адрес без схемы должен отклоняться: %v", err)
	}
}

// В production адреса по умолчанию не подставляются: незаданный сервис выключен.
func TestServiceURLsProductionDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_ENV", EnvProduction)
	t.Setenv("PUBLIC_ORIGIN", "https://shop.example.ru")
	t.Setenv("DATABASE_URL", "postgres://db")
	t.Setenv("TRUSTED_PROXIES", "172.16.0.0/12")
	t.Setenv("BACKOFFICE_TOKEN", testToken)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.OpenSearchURL != "" || c.EmbedderURL != "" || c.RerankerURL != "" {
		t.Fatalf("в production адреса по умолчанию пустые: %+v", c)
	}
}

func TestLLMConfig(t *testing.T) {
	clearEnv(t)
	l, err := LoadLLM()
	if err != nil || l.Online() || l.Mode != LLMModeOff || l.Provider != LLMNone {
		t.Fatalf("без ключа модель выключена: %+v, %v", l, err)
	}
	clearEnv(t)
	t.Setenv("LLM_API_KEY", "k")
	t.Setenv("LLM_FOLDER_ID", "b1gfolder")
	l, err = LoadLLM()
	if err != nil || !l.Enabled() || l.Model != "gpt://b1gfolder/"+defaultLLMModel {
		t.Fatalf("с ключом и каталогом модель включена: %+v, %v", l, err)
	}
	if strings.Contains(l.summary(), "b1gfolder") || strings.Contains(l.summary(), "=k") {
		t.Fatalf("сводка выдаёт каталог или ключ: %s", l.summary())
	}
	if l.IntentModel != "gpt://b1gfolder/aliceai-llm-flash" || l.FallbackModel != "" {
		t.Fatalf("модель разбора по умолчанию — Flash, резерва нет: %+v", l)
	}
	if l.JudgeModel != "gpt://b1gfolder/qwen3.6-35b-a3b" {
		t.Fatalf("судья по умолчанию — Qwen3.6 (другое семейство): %+v", l)
	}
	t.Setenv("LLM_MODEL", "aliceai-llm")
	t.Setenv("LLM_FALLBACK_MODEL", "qwen3.6-35b-a3b")
	if l, err = LoadLLM(); err != nil || l.IntentModel != "gpt://b1gfolder/aliceai-llm-flash" ||
		l.FallbackModel != "gpt://b1gfolder/qwen3.6-35b-a3b" || l.ShortModel(l.FallbackModel) != "qwen3.6-35b-a3b" {
		t.Fatalf("LLM_MODEL не меняет модель разбора, резерв дополняется каталогом: %+v, %v", l, err)
	}
	if s := l.summary(); strings.Contains(s, "b1gfolder") || !strings.Contains(s, "резерв qwen3.6-35b-a3b") {
		t.Fatalf("сводка: %s", s)
	}
	t.Setenv("LLM_MODE", "off")
	if l, err = LoadLLM(); err != nil || l.Enabled() {
		t.Fatalf("LLM_MODE=off выключает модель: %+v, %v", l, err)
	}
	for k, v := range map[string]string{"LLM_MODE": "maybe", "LLM_PROVIDER": "magic", "LLM_BASE_URL": "ftp://x"} {
		clearEnv(t)
		t.Setenv(k, v)
		if _, err := LoadLLM(); err == nil || !strings.Contains(err.Error(), k) {
			t.Fatalf("%s=%s должен отклоняться: %v", k, v, err)
		}
	}
	clearEnv(t)
	t.Setenv("LLM_MODE", "on")
	if _, err := LoadLLM(); err == nil {
		t.Fatal("LLM_MODE=on без ключа должен отклоняться")
	}
	clearEnv(t)
	t.Setenv("LLM_API_KEY", "k")
	if _, err := LoadLLM(); err == nil || !strings.Contains(err.Error(), "LLM_FOLDER_ID") {
		t.Fatalf("Yandex без каталога должен отклоняться: %v", err)
	}
}

// Защита стенда: администрирование по умолчанию только в development, лимиты читаются и проверяются.
func TestLoadOps(t *testing.T) {
	clearEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.Ops.AdminEnabled || c.Ops.RatePerMinute != defaultRatePerMinute || c.Ops.DailyBudgetRub != defaultDailyBudgetRub ||
		c.Ops.BackofficeToken != "" || c.Ops.PerIPDailyRub != defaultPerIPDailyRub || c.Ops.AgentReserveRub != defaultAgentReserveRub {
		t.Fatalf("development по умолчанию: %+v", c.Ops)
	}
	t.Setenv("APP_ENV", "production")
	t.Setenv("PUBLIC_ORIGIN", "https://example.ru")
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("TRUSTED_PROXIES", "10.0.0.1")
	t.Setenv("BACKOFFICE_TOKEN", testToken)
	t.Setenv("LLM_DAILY_BUDGET_RUB", "250,5")
	t.Setenv("RATE_LIMIT_PER_MIN", "0")
	if c, err = Load(); err != nil || c.Ops.AdminEnabled || c.Ops.DailyBudgetRub != 250.5 || c.Ops.RatePerMinute != 0 ||
		c.Ops.BackofficeToken != testToken {
		t.Fatalf("production: %+v %v", c.Ops, err)
	}
	if strings.Contains(strings.Join(c.Summary(), " "), testToken) {
		t.Fatal("токен бэк-офиса попал в сводку")
	}
	t.Setenv("BACKOFFICE_TOKEN", "short")
	t.Setenv("AGENT_BUDGET_RESERVE_RUB", "300")
	if _, err = Load(); err == nil || !strings.Contains(err.Error(), "BACKOFFICE_TOKEN") || !strings.Contains(err.Error(), "AGENT_BUDGET_RESERVE_RUB") {
		t.Fatalf("короткий токен и резерв больше бюджета: %v", err)
	}
	t.Setenv("BACKOFFICE_TOKEN", testToken)
	t.Setenv("AGENT_BUDGET_RESERVE_RUB", "")
	t.Setenv("ADMIN_ENABLED", "maybe")
	t.Setenv("LLM_DAILY_BUDGET_RUB", "-1")
	if _, err = Load(); err == nil || !strings.Contains(err.Error(), "ADMIN_ENABLED") || !strings.Contains(err.Error(), "LLM_DAILY_BUDGET_RUB") {
		t.Fatalf("ошибки настроек: %v", err)
	}
}

func TestRerankWeight(t *testing.T) {
	for raw, want := range map[string]float64{"": defaultRerankWeight, "off": 0, "0": 0, "0.3": 0.3} {
		if w, err := rerankWeight(raw); err != nil || w != want {
			t.Errorf("%q → %v %v, ожидалось %v", raw, w, err, want)
		}
	}
	for _, raw := range []string{"1.5", "-1", "abc"} {
		if _, err := rerankWeight(raw); err == nil {
			t.Errorf("%q должен быть ошибкой", raw)
		}
	}
}
