package funscript

import (
	"crypto/sha256"
	"encoding/json"
)

// StoreImportAction persists one keyframe as-is.
func StoreImportAction(action Action) StoredAction {
	pos := action.Pos
	if isWholePos(pos) {
		return StoredAction{At: action.At, Pos: float64(int(pos + 0.5))}
	}
	return StoredAction{At: action.At, Pos: roundPos(pos)}
}

// StoreImportActions persists every keyframe for import storage.
func StoreImportActions(actions []Action) []StoredAction {
	out := make([]StoredAction, len(actions))
	for i, action := range actions {
		out[i] = StoreImportAction(action)
	}
	return out
}

func normalizeBlockActions(actions []StoredAction) []StoredAction {
	out := make([]StoredAction, len(actions))
	copy(out, actions)
	return out
}

// HashBlockActions returns SHA-256 of canonical JSON for normalized at/pos pairs.
func HashBlockActions(actions []StoredAction) string {
	normalized := normalizeBlockActions(actions)
	payload, _ := json.Marshal(canonicalActions(normalized))
	sum := sha256.Sum256(payload)
	return fmtHex(sum[:])
}

func canonicalActions(actions []StoredAction) []map[string]any {
	out := make([]map[string]any, len(actions))
	for i, action := range actions {
		if isWholePos(action.Pos) {
			out[i] = map[string]any{"at": action.At, "pos": int(action.Pos + 0.5)}
		} else {
			out[i] = map[string]any{"at": action.At, "pos": roundPos(action.Pos)}
		}
	}
	return out
}

func fmtHex(data []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, len(data)*2)
	for i, b := range data {
		out[i*2] = hexdigits[b>>4]
		out[i*2+1] = hexdigits[b&0x0f]
	}
	return string(out)
}
