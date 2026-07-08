package httpapi

import (
	"encoding/json"
	"os"
	"time"
)

const agentDebugLogPath = `c:\dev\git\Handy\debug-d9c091.log`

// #region agent log
func agentDebugLog(hypothesisID, location, message string, data map[string]any) {
	payload := map[string]any{
		"sessionId":    "d9c091",
		"hypothesisId": hypothesisID,
		"location":     location,
		"message":      message,
		"data":         data,
		"timestamp":    time.Now().UnixMilli(),
	}
	line, err := json.Marshal(payload)
	if err != nil {
		return
	}
	f, err := os.OpenFile(agentDebugLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write(append(line, '\n'))
	_ = f.Close()
}

// #endregion
