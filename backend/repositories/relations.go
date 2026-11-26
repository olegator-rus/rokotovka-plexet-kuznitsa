package repositories

import (
	"context"
	"errors"
	"strings"

	"kuznica/api/domain"

	"gorm.io/gorm"
)

// Версии справочника связей (контур коммерции): config_versions.kind = relations, своя нумерация.
// Цепочка версий привязана к каталогу: импорт с другими связями начинает её заново (прежние — archived).

// relationsKind — только цепочка справочника связей.
func relationsKind(db *gorm.DB) *gorm.DB {
	return db.Where("config_versions.kind = ?", domain.ConfigKindRelations)
}

// ListRelationVersions — все версии справочника связей, новые сверху (архивные тоже).
func (r *Repos) ListRelationVersions(ctx context.Context) ([]domain.ConfigVersion, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []domain.ConfigVersion
	err := r.db.WithContext(ctx).Scopes(relationsKind).Order("version DESC").Find(&rows).Error
	return rows, err
}

// RelationsChange — одна атомарная смена справочника связей: новые версии поверх base (оптимистичная
// блокировка по максимальному номеру цепочки), решение по предложению, откат принятых предложений.
type RelationsChange struct {
	// Base — максимальный номер версии relations (архивные тоже), поверх которого строятся новые; 0 — версий нет.
	Base int
	// ArchiveActive — прежняя цепочка (не архивные версии) помечается archived: каталог сменился.
	ArchiveActive bool
	// Versions — новые версии по порядку (исходная версия из каталога и/или правка).
	Versions []*domain.ConfigVersion
	// Proposal — решаемое предложение: должно быть в статусе pending; nil — без предложения (откат).
	Proposal *domain.Proposal
	Decision *domain.Decision
	// RollBackAbove — откат: принятые relation_patch с версией выше этой становятся rolled_back (0 — не трогать).
	RollBackAbove int
	// RolledBack — решения rolled_back по откаченным предложениям (заполняется внутри).
	RolledBack []domain.Decision
}

// ApplyRelationsChange — смена справочника связей и решение одной транзакцией. Гонки: предложение уже решено —
// domain.ErrProposalDecided; версии изменились после base — domain.ErrRulesVersionConflict.
func (r *Repos) ApplyRelationsChange(ctx context.Context, ch *RelationsChange) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "kuznica_relations").Error; err != nil {
			return err
		}
		if ch.Proposal != nil {
			res := tx.Model(&domain.Proposal{}).Where("id = ? AND status = ?", ch.Proposal.ID, domain.ProposalPending).
				Updates(map[string]any{"status": ch.Proposal.Status, "decided_at": ch.Proposal.DecidedAt,
					"decided_reason": ch.Proposal.DecidedReason, "decided_comment": ch.Proposal.DecidedComment,
					"config_version": ch.Proposal.ConfigVersion, "technical_status": ch.Proposal.TechnicalStatus})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return domain.ErrProposalDecided
			}
		}
		if len(ch.Versions) > 0 {
			var latest *int
			if err := tx.Model(&domain.ConfigVersion{}).Scopes(relationsKind).Select("MAX(version)").Scan(&latest).Error; err != nil {
				return err
			}
			if (latest == nil && ch.Base != 0) || (latest != nil && *latest != ch.Base) {
				return domain.ErrRulesVersionConflict
			}
			if ch.ArchiveActive {
				if err := tx.Model(&domain.ConfigVersion{}).Scopes(relationsKind).Where("archived = ?", false).
					Update("archived", true).Error; err != nil {
					return err
				}
			}
			for _, v := range ch.Versions {
				v.Kind = domain.ConfigKindRelations
				if err := tx.Create(v).Error; err != nil {
					if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "23505") {
						return domain.ErrRulesVersionConflict
					}
					return err
				}
			}
		}
		if ch.RollBackAbove > 0 {
			var undone []domain.Proposal
			if err := tx.Where("status = ? AND action = ? AND config_version > ?", domain.ProposalAccepted,
				domain.ActionRelationPatch, ch.RollBackAbove).Find(&undone).Error; err != nil {
				return err
			}
			for i := range undone {
				p := &undone[i]
				if err := tx.Model(p).Update("status", domain.ProposalRolledBack).Error; err != nil {
					return err
				}
				d := domain.Decision{ProposalID: p.ID, Decision: domain.ProposalRolledBack, Reason: "rollback",
					ClusterKey: p.ClusterKey, ClusterLabel: p.ClusterLabel, Diagnosis: p.Diagnosis, PatchKeys: p.PatchKeys}
				if ch.Decision != nil {
					d.Comment = ch.Decision.Comment
				}
				if err := tx.Create(&d).Error; err != nil {
					return err
				}
				ch.RolledBack = append(ch.RolledBack, d)
			}
		}
		if ch.Decision != nil && ch.Proposal != nil {
			return tx.Create(ch.Decision).Error
		}
		return nil
	})
}

// commerceEvalSet — строка evaluations с результатом коммерческой лестницы целиком (Metrics = весь ответ).
const commerceEvalSet = "commerce"

// SaveCommerceEvaluation — результат коммерческой лестницы одной строкой evaluations (eval_set = commerce).
func (r *Repos) SaveCommerceEvaluation(ctx context.Context, row *domain.Evaluation) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	row.Set = commerceEvalSet
	return r.db.WithContext(ctx).Create(row).Error
}

// LatestCommerceEvaluation — последний результат коммерческой лестницы набора set (в RunID — «set:run»); нет — nil.
func (r *Repos) LatestCommerceEvaluation(ctx context.Context, set string) (*domain.Evaluation, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var row domain.Evaluation
	err := r.db.WithContext(ctx).Where("eval_set = ? AND config = ?", commerceEvalSet, set).
		Order("created_at DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}
