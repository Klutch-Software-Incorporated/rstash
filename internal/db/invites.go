package db

import (
	"context"
	"database/sql"
	"fmt"

	"gosilo/internal/model"
)

// CreateInviteCode generates a new invite code (8-byte hex = 16 chars).
func CreateInviteCode(ctx context.Context, q Querier, createdBy int64) (*model.InviteCode, error) {
	code, err := RandomHex(8)
	if err != nil {
		return nil, fmt.Errorf("generate invite code: %w", err)
	}

	var inv model.InviteCode
	err = q.QueryRowContext(ctx,
		`INSERT INTO invite_codes (code, created_by)
		 VALUES (?, ?)
		 RETURNING code, created_by, used_by, created_at, used_at`,
		code, createdBy,
	).Scan(&inv.Code, &inv.CreatedBy, &inv.UsedBy, &inv.CreatedAt, &inv.UsedAt)
	if err != nil {
		return nil, fmt.Errorf("create invite code: %w", err)
	}
	return &inv, nil
}

// GetInviteCode returns the invite code with the given code, or nil if not found.
func GetInviteCode(ctx context.Context, q Querier, code string) (*model.InviteCode, error) {
	var inv model.InviteCode
	err := q.QueryRowContext(ctx,
		"SELECT code, created_by, used_by, created_at, used_at FROM invite_codes WHERE code = ?",
		code,
	).Scan(&inv.Code, &inv.CreatedBy, &inv.UsedBy, &inv.CreatedAt, &inv.UsedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get invite code: %w", err)
	}
	return &inv, nil
}

// RedeemInviteCode marks an invite as used by the given user. Returns an error
// if the invite doesn't exist or is already used.
func RedeemInviteCode(ctx context.Context, q Querier, code string, usedBy int64) error {
	res, err := q.ExecContext(ctx,
		`UPDATE invite_codes SET used_by = ?, used_at = datetime('now')
		 WHERE code = ? AND used_by IS NULL`,
		usedBy, code,
	)
	if err != nil {
		return fmt.Errorf("redeem invite code: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("redeem invite code rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("invite code not found or already used")
	}
	return nil
}

// ListInviteCodes returns all invite codes ordered by created_at desc.
func ListInviteCodes(ctx context.Context, q Querier) ([]*model.InviteCode, error) {
	rows, err := q.QueryContext(ctx,
		"SELECT code, created_by, used_by, created_at, used_at FROM invite_codes ORDER BY created_at DESC")
	if err != nil {
		return nil, fmt.Errorf("list invite codes: %w", err)
	}
	defer rows.Close()

	var invites []*model.InviteCode
	for rows.Next() {
		var inv model.InviteCode
		if err := rows.Scan(&inv.Code, &inv.CreatedBy, &inv.UsedBy, &inv.CreatedAt, &inv.UsedAt); err != nil {
			return nil, fmt.Errorf("scan invite code: %w", err)
		}
		invites = append(invites, &inv)
	}
	return invites, rows.Err()
}

// DeleteInviteCode deletes an unused invite code.
func DeleteInviteCode(ctx context.Context, q Querier, code string) error {
	_, err := q.ExecContext(ctx, "DELETE FROM invite_codes WHERE code = ? AND used_by IS NULL", code)
	if err != nil {
		return fmt.Errorf("delete invite code: %w", err)
	}
	return nil
}
