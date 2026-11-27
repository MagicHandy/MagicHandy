package chat

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	lookupRowLimit  = 16
	lookupByteLimit = 4096
)

// HistoryLookup is an exclusive, read-only model decision. Matching terms
// are literal strings bound to SQL parameters, never a model-authored query.
type HistoryLookup struct {
	Action              string        `json:"action"`
	Scope               string        `json:"scope"`
	Terms               []string      `json:"terms"`
	Facts               []historyFact `json:"facts"`
	Count               int           `json:"count,omitempty"`
	FollowingAdjustment *bool         `json:"following_adjustment,omitempty"`
}

type historyFact struct {
	Detail string `json:"detail"`
	Kind   string `json:"kind"`
}

// HistorySource holds a complete committed user message and its stable ID.
type HistorySource struct {
	ID   int64  `json:"id"`
	Text string `json:"text"`
}

// HistoryReader reads evidence available to one admitted interactive turn.
type HistoryReader func(context.Context, HistoryLookup) ([]HistorySource, error)

// A pending message with an older sequence can commit after the admitted user
// turn. Bound both its sequence and its publication revision to that turn.
const lookupWhere = ` FROM messages m WHERE m.session_id = ?1 AND m.role = 'user' AND m.committed = 1
	AND (?2 = 0 OR (m.seq < ?2 AND m.revision < (SELECT revision FROM messages WHERE session_id = ?1 AND seq = ?2 AND committed = 1)))`

const openingLookupSQL = `SELECT seq, substr(CAST(content AS BLOB), 1, ?3 + 1)` + lookupWhere + `
	AND EXISTS (SELECT 1 FROM chat_sessions WHERE id = ?1 AND pruned_revision = 0 AND reset_revision = 0)
	ORDER BY seq LIMIT ?4`

const matchingLookupSQL = `WITH terms(term, weight) AS (
	SELECT value, 4 FROM json_each(?5) UNION ALL SELECT value, 1 FROM json_each(?6)
	), ranked AS (SELECT m.seq, m.content,
	(SELECT SUM(weight) FROM terms WHERE instr(lower(m.content), lower(term)) > 0) AS score` + lookupWhere + `)
	SELECT seq, substr(CAST(content AS BLOB), 1, ?3 + 1) FROM ranked WHERE score > 0
	ORDER BY score DESC, seq DESC LIMIT ?4`

// UserHistoryBefore binds the reader to the admitted conversation and turn.
// The preview uses zero because it never appends a user row.
func (l *MessageLog) UserHistoryBefore(sessionID string, beforeSeq int64) HistoryReader {
	return func(ctx context.Context, lookup HistoryLookup) ([]HistorySource, error) {
		if !lookup.valid() {
			return nil, errors.New("invalid saved-message lookup")
		}
		// Once pruning or a committed deletion obscures the beginning, a
		// surviving named detail can still be verified by a matching lookup.
		query := openingLookupSQL
		args := []any{sessionID, beforeSeq, lookupByteLimit, lookupRowLimit}
		if lookup.Scope == "matching" {
			query = matchingLookupSQL
			var words []string
			for _, term := range lookup.Terms {
				parts := strings.FieldsFunc(term, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
				for _, word := range parts {
					if len(parts) > 1 && utf8.RuneCountInString(word) >= 3 && !slices.Contains(words, word) {
						words = append(words, word)
					}
				}
			}
			phrasesJSON, _ := json.Marshal(lookup.Terms)
			wordsJSON, _ := json.Marshal(words)
			args = append(args, string(phrasesJSON), string(wordsJSON))
		}
		rows, err := l.db.SQL().QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		var sources []HistorySource
		remaining := lookupByteLimit
		for rows.Next() {
			var source HistorySource
			if err := rows.Scan(&source.ID, &source.Text); err != nil {
				return nil, err
			}
			if len(source.Text) > remaining {
				if lookup.Scope != "matching" {
					break // preserve a continuous prefix of whole user turns
				}
				continue
			}
			sources = append(sources, source)
			remaining -= len(source.Text)
		}
		return sources, rows.Err()
	}
}

func (q HistoryLookup) valid() bool {
	if q.Scope == "opening_paces" {
		return q.Action == "read_user_history" && q.Count > 0 && q.Count <= 8 && q.FollowingAdjustment != nil && q.Facts == nil && q.Terms == nil
	}
	if q.Action != "read_user_history" || q.Terms == nil || len(q.Facts) == 0 || len(q.Facts) > 8 ||
		slices.ContainsFunc(q.Facts, func(fact historyFact) bool {
			return strings.TrimSpace(fact.Detail) == "" || len(fact.Detail) > 180 || (fact.Kind != "number" && fact.Kind != "text")
		}) {
		return false
	}
	return (q.Scope == "opening" || q.Scope == "matching" && len(q.Terms) > 0) && len(q.Terms) <= 4 && !slices.ContainsFunc(q.Terms, func(term string) bool {
		return strings.TrimSpace(term) == "" || len(term) > 80
	})
}
