package handlers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// Сессии покупателя выдаёт сервер (ревью-2 n3): GET /api/meta отдаёт подписанный токен «s-<16 hex>.<HMAC>», фронт
// шлёт его в session_id поиска и событий (или заголовком X-Session). В журнал пишется только номер сессии из
// токена с верной подписью; самодельный session_id сохраняется как «без сессии», поэтому подбором номеров
// нельзя набить «различные сессии» и доказательства кликами для агента. Ключ подписи — из BACKOFFICE_TOKEN
// (сессии переживают перезапуск); без токена (development) — случайный на процесс.

// SessionHeader — заголовок с токеном сессии: в запросе к /api/meta — продлить текущую сессию.
const SessionHeader = "X-Session"

const (
	sessionPrefix  = "s-"
	sessionIDHex   = 16 // 64 бита номера сессии
	sessionSigHex  = 32 // 128 бит подписи
	sessionKeySalt = "plexet-session-v1"
)

// newSessionKey — ключ HMAC сессий: производный от токена бэк-офиса или случайный.
func newSessionKey(backofficeToken string) []byte {
	if backofficeToken != "" {
		m := hmac.New(sha256.New, []byte(backofficeToken))
		m.Write([]byte(sessionKeySalt))
		return m.Sum(nil)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	return key
}

// sessionSig — подпись номера сессии.
func (h *Handler) sessionSig(id string) string {
	m := hmac.New(sha256.New, h.sessionKey)
	m.Write([]byte(id))
	return hex.EncodeToString(m.Sum(nil))[:sessionSigHex]
}

// issueSession — новый токен сессии.
func (h *Handler) issueSession() string {
	b := make([]byte, sessionIDHex/2)
	_, _ = rand.Read(b)
	id := sessionPrefix + hex.EncodeToString(b)
	return id + "." + h.sessionSig(id)
}

// verifySession — номер сессии из токена с верной подписью; "" — токена нет или он не выдан этим сервером.
func (h *Handler) verifySession(token string) string {
	id, sig, ok := strings.Cut(token, ".")
	if !ok || len(id) != len(sessionPrefix)+sessionIDHex || !strings.HasPrefix(id, sessionPrefix) || len(sig) != sessionSigHex {
		return ""
	}
	if _, err := hex.DecodeString(id[len(sessionPrefix):]); err != nil {
		return ""
	}
	if !hmac.Equal([]byte(sig), []byte(h.sessionSig(id))) {
		return ""
	}
	return id
}

// requestSession — номер сессии запроса: токен из тела (session_id), иначе из заголовка X-Session.
func (h *Handler) requestSession(c *fiber.Ctx, body string) string {
	if body == "" {
		body = c.Get(SessionHeader)
	}
	return h.verifySession(body)
}
