package chat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// MessageContentChunkBytes bounds each materialized download chunk. No database
// connection or read transaction is retained while its bytes reach a client.
const MessageContentChunkBytes = 64 << 10

// ErrChatMessageNotFound covers missing, pending and superseded message identities.
var ErrChatMessageNotFound = errors.New("chat message is no longer available; reload the conversation")

// MessageContentChunk contains at most one bounded slice of canonical UTF-8 bytes.
type MessageContentChunk struct {
	Data       []byte
	TotalBytes int64
}

// ReadMessageContentChunkContext releases the database before returning the chunk.
func (l *MessageLog) ReadMessageContentChunkContext(ctx context.Context, sessionID string, seq, revision, offset int64) (MessageContentChunk, error) {
	var chunk MessageContentChunk
	if sessionID == "" || seq <= 0 || revision <= 0 || offset < 0 || offset == int64(^uint64(0)>>1) {
		return chunk, ErrChatMessageNotFound
	}
	err := l.db.SQL().QueryRowContext(ctx, `SELECT substr(CAST(content AS BLOB), ?, ?), length(CAST(content AS BLOB))
		FROM messages WHERE session_id = ? AND seq = ? AND revision = ? AND committed = 1`,
		offset+1, MessageContentChunkBytes, sessionID, seq, revision).Scan(&chunk.Data, &chunk.TotalBytes)
	if errors.Is(err, sql.ErrNoRows) {
		return chunk, ErrChatMessageNotFound
	}
	if err != nil {
		return chunk, fmt.Errorf("read chat message content: %w", err)
	}
	return chunk, nil
}
