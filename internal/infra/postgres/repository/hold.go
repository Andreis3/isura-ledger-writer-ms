package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	domainhold "github.com/andreis3/isura-ledger-ms/internal/domain/hold"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// HoldRepository persists current hold state, immutable history, and releases.
type HoldRepository struct{ db database.Querier }

var _ domainhold.Repository = (*HoldRepository)(nil)

// NewHoldRepository constructs the PostgreSQL hold adapter.
func NewHoldRepository(db database.Querier) *HoldRepository { return &HoldRepository{db: db} }

func (r *HoldRepository) Save(ctx context.Context, instruction *domainhold.Instruction) error {
	value, err := model.ToHoldModel(instruction)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = resolveDB(ctx, r.db).Exec(ctx, `
		INSERT INTO holds (id, account_id, currency, origin, reference, type, requested_amount,
			allocated_amount, pending_amount, released_amount, status, policy_version,
			request_fingerprint, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$14)`,
		value.ID, value.AccountID, value.Currency, value.Origin, value.Reference, value.Type,
		value.RequestedAmount, value.AllocatedAmount, value.PendingAmount, value.ReleasedAmount,
		value.Status, nullableString(value.PolicyVersion), value.RequestFingerprint, now)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %v", domainhold.ErrHoldConflict, err)
		}
		return fmt.Errorf("save hold: %w", err)
	}
	return nil
}

func (r *HoldRepository) Update(ctx context.Context, instruction *domainhold.Instruction) error {
	value, err := model.ToHoldModel(instruction)
	if err != nil {
		return err
	}
	result, err := resolveDB(ctx, r.db).Exec(ctx, `
		UPDATE holds SET allocated_amount=$2, pending_amount=$3, released_amount=$4,
			status=$5, policy_version=$6, updated_at=$7 WHERE id=$1`,
		value.ID, value.AllocatedAmount, value.PendingAmount, value.ReleasedAmount,
		value.Status, nullableString(value.PolicyVersion), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("update hold: %w", err)
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *HoldRepository) FindByIdentity(ctx context.Context, origin, reference string) (*domainhold.Instruction, error) {
	return r.find(ctx, `origin=$1 AND reference=$2`, origin, reference)
}

func (r *HoldRepository) FindByID(ctx context.Context, id string) (*domainhold.Instruction, error) {
	return r.find(ctx, `id=$1`, id)
}

func (r *HoldRepository) find(ctx context.Context, predicate string, args ...any) (*domainhold.Instruction, error) {
	query := holdSelect + ` WHERE ` + predicate
	value, err := scanHold(resolveDB(ctx, r.db).QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find hold: %w", err)
	}
	return model.ToHoldDomain(value)
}

func (r *HoldRepository) FindEffectiveByAccount(ctx context.Context, accountID string, currency money.Currency) ([]domainhold.Instruction, error) {
	rows, err := resolveDB(ctx, r.db).Query(ctx, holdSelect+`
		WHERE account_id=$1 AND currency=$2 AND allocated_amount > released_amount
		ORDER BY created_at, id`, accountID, string(currency))
	if err != nil {
		return nil, fmt.Errorf("find effective holds: %w", err)
	}
	defer rows.Close()
	result := make([]domainhold.Instruction, 0)
	for rows.Next() {
		value, scanErr := scanHold(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan effective hold: %w", scanErr)
		}
		instruction, convertErr := model.ToHoldDomain(value)
		if convertErr != nil {
			return nil, convertErr
		}
		result = append(result, *instruction)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate effective holds: %w", err)
	}
	return result, nil
}

func (r *HoldRepository) SaveHistory(ctx context.Context, history domainhold.History) error {
	requested, allocated, pending, released, err := historyAmounts(history)
	if err != nil {
		return err
	}
	conflicts, err := model.ConflictIDsJSON(history.ConflictIDs)
	if err != nil {
		return fmt.Errorf("encode hold conflict IDs: %w", err)
	}
	createdAt := history.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err = resolveDB(ctx, r.db).Exec(ctx, `
		INSERT INTO hold_history (id, hold_id, event_type, requested_amount, allocated_delta,
			pending_amount, released_delta, policy_version, idempotency_key, reason, conflict_ids, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, history.ID, history.HoldID,
		history.EventType, requested, allocated, pending, released,
		nullableString(history.PolicyVersion), nullableString(history.IdempotencyKey),
		nullableString(history.Reason), conflicts, createdAt)
	if err != nil {
		return fmt.Errorf("save hold history: %w", err)
	}
	return nil
}

func (r *HoldRepository) FindHistory(ctx context.Context, holdID string) ([]domainhold.History, error) {
	rows, err := resolveDB(ctx, r.db).Query(ctx, `SELECT h.id, h.hold_id, h.event_type, h.requested_amount,
		h.allocated_delta, h.pending_amount, h.released_delta, h.policy_version, h.idempotency_key, h.reason,
		h.conflict_ids, h.created_at, a.currency FROM hold_history h JOIN holds a ON a.id=h.hold_id
		WHERE h.hold_id=$1 ORDER BY h.created_at, h.id`, holdID)
	if err != nil {
		return nil, fmt.Errorf("find hold history: %w", err)
	}
	defer rows.Close()
	history := make([]domainhold.History, 0)
	for rows.Next() {
		var value model.HoldHistory
		var policyVersion, idempotencyKey, reason pgtype.Text
		var currency string
		if err := rows.Scan(&value.ID, &value.HoldID, &value.EventType, &value.RequestedAmount,
			&value.AllocatedDelta, &value.PendingAmount, &value.ReleasedDelta, &policyVersion,
			&idempotencyKey, &reason, &value.ConflictIDs, &value.CreatedAt, &currency); err != nil {
			return nil, fmt.Errorf("scan hold history: %w", err)
		}
		currencyCode := money.Currency(currency)
		requested, err := money.NewMoney(value.RequestedAmount, currencyCode)
		if err != nil {
			return nil, err
		}
		allocated, err := money.NewMoney(value.AllocatedDelta, currencyCode)
		if err != nil {
			return nil, err
		}
		pending, err := money.NewMoney(value.PendingAmount, currencyCode)
		if err != nil {
			return nil, err
		}
		released, err := money.NewMoney(value.ReleasedDelta, currencyCode)
		if err != nil {
			return nil, err
		}
		var conflictIDs []string
		if err := json.Unmarshal(value.ConflictIDs, &conflictIDs); err != nil {
			return nil, fmt.Errorf("decode hold conflict IDs: %w", err)
		}
		history = append(history, domainhold.History{ID: value.ID, HoldID: value.HoldID, EventType: value.EventType,
			Requested: requested, AllocatedDelta: allocated, Pending: pending, ReleasedDelta: released,
			PolicyVersion: optionalText(policyVersion), IdempotencyKey: optionalText(idempotencyKey),
			Reason: optionalText(reason), ConflictIDs: conflictIDs, CreatedAt: value.CreatedAt})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hold history: %w", err)
	}
	return history, nil
}

func (r *HoldRepository) FindRelease(ctx context.Context, key string) (*domainhold.Release, error) {
	var value domainhold.Release
	var amount int64
	var currency string
	err := resolveDB(ctx, r.db).QueryRow(ctx, `SELECT id, hold_id, idempotency_key, amount,
		authorization_reference, request_fingerprint, created_at,
		(SELECT currency FROM holds WHERE holds.id=hold_releases.hold_id)
		FROM hold_releases WHERE idempotency_key=$1`, key).Scan(&value.ID, &value.HoldID,
		&value.IdempotencyKey, &amount, &value.AuthorizationReference, &value.RequestFingerprint,
		&value.CreatedAt, &currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find hold release: %w", err)
	}
	value.Amount, err = money.NewMoney(amount, money.Currency(currency))
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (r *HoldRepository) SaveRelease(ctx context.Context, release domainhold.Release) error {
	if !release.Amount.IsPositive() || release.ID == "" || release.HoldID == "" || release.IdempotencyKey == "" || release.AuthorizationReference == "" || len(release.RequestFingerprint) != 64 {
		return fmt.Errorf("invalid hold release")
	}
	createdAt := release.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := resolveDB(ctx, r.db).Exec(ctx, `INSERT INTO hold_releases
		(id, hold_id, idempotency_key, amount, authorization_reference, request_fingerprint, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, release.ID, release.HoldID, release.IdempotencyKey,
		release.Amount.Amount(), release.AuthorizationReference, release.RequestFingerprint, createdAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %v", domainhold.ErrReleaseConflict, err)
		}
		return fmt.Errorf("save hold release: %w", err)
	}
	return nil
}

const holdSelect = `SELECT id, account_id, currency, origin, reference, type, requested_amount,
	allocated_amount, pending_amount, released_amount, status, policy_version, request_fingerprint,
	created_at, updated_at FROM holds`

type rowScanner interface{ Scan(dest ...any) error }

func scanHold(row rowScanner) (model.Hold, error) {
	var value model.Hold
	var policyVersion pgtype.Text
	err := row.Scan(&value.ID, &value.AccountID, &value.Currency, &value.Origin, &value.Reference,
		&value.Type, &value.RequestedAmount, &value.AllocatedAmount, &value.PendingAmount,
		&value.ReleasedAmount, &value.Status, &policyVersion, &value.RequestFingerprint,
		&value.CreatedAt, &value.UpdatedAt)
	if err != nil {
		return model.Hold{}, err
	}
	value.PolicyVersion = optionalText(policyVersion)
	return value, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func optionalText(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func historyAmounts(history domainhold.History) (int64, int64, int64, int64, error) {
	requested, allocated, pending, released := history.Requested, history.AllocatedDelta, history.Pending, history.ReleasedDelta
	currency := requested.Currency()
	if !currency.IsValid() {
		currency = allocated.Currency()
	}
	if !currency.IsValid() {
		currency = pending.Currency()
	}
	if !currency.IsValid() {
		currency = released.Currency()
	}
	for _, amount := range []money.Money{requested, allocated, pending, released} {
		if !amount.Currency().IsValid() || amount.Currency() != currency || amount.IsNegative() {
			return 0, 0, 0, 0, fmt.Errorf("invalid hold history amount")
		}
	}
	if history.ID == "" || history.HoldID == "" || history.EventType == "" {
		return 0, 0, 0, 0, fmt.Errorf("invalid hold history identity")
	}
	return requested.Amount(), allocated.Amount(), pending.Amount(), released.Amount(), nil
}
