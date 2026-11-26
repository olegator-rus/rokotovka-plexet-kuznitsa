package in

// AgentRunRequest — POST /api/agent/runs. Пустые поля — значения по умолчанию (30 дней, 5 кластеров, manual).
type AgentRunRequest struct {
	Days        int    `json:"days"`
	MaxClusters int    `json:"max_clusters"`
	Trigger     string `json:"trigger"`
	// Mode — search (по умолчанию: кластеры неудачных запросов) | commerce (справочник связей, контур коммерции).
	Mode string `json:"mode"`
	// Query — запуск «Разобрать» на одном запросе (контур доводки и автономии): триггер manual, кластер из одного запроса.
	Query string `json:"query"`
}

// SuppressRemoveRequest — POST /api/agent/memory/suppress/{id}/remove: причина обязательна.
type SuppressRemoveRequest struct {
	Reason string `json:"reason"`
}

// AgentSettingsRequest — POST /api/agent/settings: режим «только чтение».
type AgentSettingsRequest struct {
	ReadOnly *bool `json:"read_only"`
}

// ApproveRequest — POST /api/proposals/{id}/approve.
type ApproveRequest struct {
	Comment string `json:"comment"`
}

// RejectRequest — POST /api/proposals/{id}/reject: причина обязательна.
type RejectRequest struct {
	Reason  string `json:"reason"`
	Comment string `json:"comment"`
}

// RollbackRequest — POST /api/configs/rollback.
type RollbackRequest struct {
	ToVersion int    `json:"to_version"`
	Comment   string `json:"comment"`
}
