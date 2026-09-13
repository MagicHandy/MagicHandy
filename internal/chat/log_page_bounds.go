package chat

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

// MessagePageMaxBytes includes the HTTP envelope. Preview limits apply only to
// history reads; canonical content and model prompt policies remain unchanged.
const (
	MessagePageMaxBytes       = 256 << 10
	MessagePreviewBytes       = 16 << 10
	messageDiagnosticsBytes   = 8 << 10
	messagePageEnvelopeBytes  = 4096
	messageSpeechReserveBytes = 1024
)

var messagePageColumns = fmt.Sprintf(`SELECT seq, role, substr(CAST(content AS BLOB), 1, %d),
	COALESCE(substr(CAST(client_id AS BLOB), 1, 256), ''),
	CASE WHEN length(CAST(diagnostics_json AS BLOB)) <= %d THEN diagnostics_json ELSE '{}' END,
	substr(created_at, 1, 64), revision, length(CAST(content AS BLOB)),
	length(CAST(diagnostics_json AS BLOB)) > %d`, MessagePreviewBytes, messageDiagnosticsBytes, messageDiagnosticsBytes)

var (
	messagePageSequenceQuery = messagePageColumns + ` FROM messages WHERE session_id = ? AND committed = 1
		AND seq > ? AND revision <= ? ORDER BY seq ASC LIMIT ?`
	messagePageRevisionQuery = messagePageColumns + ` FROM messages WHERE session_id = ? AND committed = 1
		AND revision > ? AND revision <= ? ORDER BY revision ASC, seq ASC LIMIT ?`
)

func scanBoundedMessagePage(rows *sql.Rows, limit int) ([]LogMessage, bool, error) {
	messages := make([]LogMessage, 0)
	used := messagePageEnvelopeBytes
	for rows.Next() {
		if len(messages) >= limit {
			return messages, true, nil
		}
		var message LogMessage
		var diagnostics string
		if err := rows.Scan(&message.Seq, &message.Role, &message.Content, &message.ClientID, &diagnostics,
			&message.CreatedAt, &message.Revision, &message.ContentBytes, &message.DiagnosticsOmitted); err != nil {
			return nil, false, fmt.Errorf("scan chat preview: %w", err)
		}
		message.Content = completeUTF8Prefix(message.Content)
		message.ClientID = completeUTF8Prefix(message.ClientID)
		message.ContentTruncated = message.ContentBytes > int64(len(message.Content))
		if diagnostics != "" && diagnostics != "{}" {
			if err := json.Unmarshal([]byte(diagnostics), &message.Diagnostics); err != nil {
				return nil, false, fmt.Errorf("decode chat preview diagnostics: %w", err)
			}
		}
		encoded, err := json.Marshal(message)
		if err != nil {
			return nil, false, err
		}
		// Speech is attached under the publication gate by HTTP after the short
		// read transaction. Reserve its bounded ID and each row's comma here.
		cost := len(encoded) + messageSpeechReserveBytes + 1
		if used+cost > MessagePageMaxBytes {
			if len(messages) == 0 {
				return nil, false, errors.New("chat preview exceeds page budget")
			}
			return messages, true, nil
		}
		messages = append(messages, message)
		used += cost
	}
	return messages, false, rows.Err()
}

func completeUTF8Prefix(value string) string {
	// Stored app text is UTF-8. Only a byte-prefix cut can split the final rune.
	start := len(value) - 1
	for start > 0 && !utf8.RuneStart(value[start]) {
		start--
	}
	if start >= 0 && !utf8.FullRuneInString(value[start:]) {
		return value[:start]
	}
	return value
}
