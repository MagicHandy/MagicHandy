package patterns

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"
)

const feedbackWeightStep = 0.15

// ApplyFeedback records one thumbs adjustment and returns the visible row.
func (l *Library) ApplyFeedback(patternID string, rating int) (Feedback, Pattern, error) {
	if rating != -1 && rating != 1 {
		return Feedback{}, Pattern{}, fmt.Errorf("%w: feedback rating must be -1 or 1", ErrInvalidContent)
	}
	ctx := context.Background()
	var feedback Feedback
	var pattern Pattern
	err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		pattern, err = queryPattern(ctx, tx, patternID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPatternNotFound
		}
		if err != nil {
			return err
		}
		beforeWeight, beforeEnabled := pattern.Weight, pattern.Enabled
		pattern.Weight = mathClamp(pattern.Weight+float64(rating)*feedbackWeightStep, 0.1, 3)
		autoDisable, err := l.autoDisableTx(ctx, tx)
		if err != nil {
			return err
		}
		if rating < 0 && autoDisable && pattern.Weight <= 0.25 {
			pattern.Enabled = false
		}
		pattern.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `
			UPDATE patterns SET weight = ?, enabled = ?, updated_at = ? WHERE id = ?
		`, pattern.Weight, boolInt(pattern.Enabled), pattern.UpdatedAt, pattern.ID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `
			INSERT INTO pattern_feedback(pattern_id, rating, weight_before, weight_after,
				enabled_before, enabled_after, created_at)
			VALUES(?, ?, ?, ?, ?, ?, ?)
		`, pattern.ID, rating, beforeWeight, pattern.Weight, boolInt(beforeEnabled),
			boolInt(pattern.Enabled), pattern.UpdatedAt)
		if err != nil {
			return err
		}
		feedback.ID, err = result.LastInsertId()
		feedback.PatternID = pattern.ID
		feedback.Rating = rating
		feedback.WeightBefore, feedback.WeightAfter = beforeWeight, pattern.Weight
		feedback.EnabledBefore, feedback.EnabledAfter = beforeEnabled, pattern.Enabled
		feedback.CreatedAt = pattern.UpdatedAt
		return err
	})
	if err != nil {
		return Feedback{}, Pattern{}, err
	}
	return feedback, pattern, nil
}

// UndoFeedback restores the exact prior weight/enablement when no newer
// feedback exists for that pattern.
func (l *Library) UndoFeedback(id int64) (Feedback, Pattern, error) {
	ctx := context.Background()
	var feedback Feedback
	var pattern Pattern
	var patternID string
	err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		var enabledBefore, enabledAfter, reverted int
		err := tx.QueryRowContext(ctx, `
			SELECT id, pattern_id, rating, weight_before, weight_after, enabled_before,
			       enabled_after, reverted, created_at, reverted_at
			FROM pattern_feedback WHERE id = ?
		`, id).Scan(&feedback.ID, &patternID, &feedback.Rating, &feedback.WeightBefore,
			&feedback.WeightAfter, &enabledBefore, &enabledAfter, &reverted,
			&feedback.CreatedAt, &feedback.RevertedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrFeedbackNotFound
		}
		if err != nil {
			return err
		}
		feedback.PatternID = patternID
		feedback.EnabledBefore, feedback.EnabledAfter = enabledBefore != 0, enabledAfter != 0
		feedback.Reverted = reverted != 0
		if feedback.Reverted {
			return ErrFeedbackReverted
		}
		var newer int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM pattern_feedback
			WHERE pattern_id = ? AND id > ? AND reverted = 0
		`, patternID, id).Scan(&newer); err != nil {
			return err
		}
		if newer > 0 {
			return ErrFeedbackOrder
		}
		current, err := queryPattern(ctx, tx, patternID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPatternNotFound
		}
		if err != nil {
			return err
		}
		if math.Abs(current.Weight-feedback.WeightAfter) > 0.0001 || current.Enabled != feedback.EnabledAfter {
			return ErrFeedbackOrder
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		pattern = current
		pattern.Weight = feedback.WeightBefore
		pattern.Enabled = feedback.EnabledBefore
		pattern.UpdatedAt = now
		if _, err := tx.ExecContext(ctx, `
			UPDATE patterns SET weight = ?, enabled = ?, updated_at = ? WHERE id = ?
		`, feedback.WeightBefore, boolInt(feedback.EnabledBefore), now, patternID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE pattern_feedback SET reverted = 1, reverted_at = ? WHERE id = ?
		`, now, id); err != nil {
			return err
		}
		feedback.Reverted, feedback.RevertedAt = true, now
		return nil
	})
	if err != nil {
		return Feedback{}, Pattern{}, err
	}
	return feedback, pattern, nil
}

// FeedbackHistory returns newest entries first.
func (l *Library) FeedbackHistory(limit int) ([]Feedback, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := l.db.SQL().QueryContext(context.Background(), `
		SELECT id, pattern_id, rating, weight_before, weight_after, enabled_before,
		       enabled_after, reverted, created_at, reverted_at
		FROM pattern_feedback ORDER BY id DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	feedback := make([]Feedback, 0)
	for rows.Next() {
		var item Feedback
		var enabledBefore, enabledAfter, reverted int
		if err := rows.Scan(&item.ID, &item.PatternID, &item.Rating, &item.WeightBefore,
			&item.WeightAfter, &enabledBefore, &enabledAfter, &reverted,
			&item.CreatedAt, &item.RevertedAt); err != nil {
			return nil, err
		}
		item.EnabledBefore, item.EnabledAfter = enabledBefore != 0, enabledAfter != 0
		item.Reverted = reverted != 0
		feedback = append(feedback, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return feedback, nil
}

// AutoDisable reports the explicit opt-in setting.
func (l *Library) AutoDisable() (bool, error) {
	var value string
	err := l.db.SQL().QueryRowContext(context.Background(), `
		SELECT value FROM app_kv WHERE key = ?
	`, autoDisableKey).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return parseAutoDisable(value)
}

// SetAutoDisable updates the opt-in without changing any pattern immediately.
func (l *Library) SetAutoDisable(enabled bool) error {
	ctx := context.Background()
	return l.db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO app_kv(key, value, updated_at) VALUES(?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
		`, autoDisableKey, strconv.FormatBool(enabled), time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
}

func (l *Library) autoDisableTx(ctx context.Context, tx *sql.Tx) (bool, error) {
	var value string
	err := tx.QueryRowContext(ctx, `SELECT value FROM app_kv WHERE key = ?`, autoDisableKey).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return parseAutoDisable(value)
}

func parseAutoDisable(value string) (bool, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid auto-disable preference %q", value)
	}
}
