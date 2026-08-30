package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	setupResultSchema    = 1
	setupResultFileName  = "setup-last-install.json"
	setupResultFileLimit = 64 * 1024
	setupResultTextLimit = 512
)

type persistedSetupResult struct {
	SchemaVersion int      `json:"schema_version"`
	Job           setupJob `json:"job"`
}

func (m *setupManager) setupResultPath() string {
	return filepath.Join(m.dataDir, setupResultFileName)
}

func (m *setupManager) loadPersistedSetupJob() {
	file, err := os.Open(m.setupResultPath()) // #nosec G304 -- fixed app-owned data file.
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || info.Size() <= 0 || info.Size() > setupResultFileLimit {
		return
	}
	var record persistedSetupResult
	decoder := json.NewDecoder(io.LimitReader(file, setupResultFileLimit))
	if err := decoder.Decode(&record); err != nil || record.SchemaVersion != setupResultSchema {
		return
	}
	if !terminalSetupStatus(record.Job.Status) {
		return
	}
	record.Job = sanitizePersistedSetupJob(record.Job)
	m.job = &setupJobState{setupJob: record.Job}
}

func (m *setupManager) persistSetupJob(job setupJob) {
	job = sanitizePersistedSetupJob(job)
	job.Output = ""
	record := persistedSetupResult{SchemaVersion: setupResultSchema, Job: job}
	data, err := json.Marshal(record)
	if err != nil || len(data) > setupResultFileLimit {
		return
	}
	if err := os.MkdirAll(m.dataDir, 0o750); err != nil {
		m.logSetupPersistenceFailure(err)
		return
	}
	path := m.setupResultPath()
	temporary, err := os.CreateTemp(m.dataDir, ".setup-last-install-*")
	if err != nil {
		m.logSetupPersistenceFailure(err)
		return
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	err = temporary.Chmod(0o600)
	if err == nil {
		_, err = temporary.Write(data)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temporaryPath, path)
		if err != nil {
			if removeErr := os.Remove(path); removeErr == nil || errors.Is(removeErr, os.ErrNotExist) {
				err = os.Rename(temporaryPath, path)
			}
		}
	}
	if err != nil {
		m.logSetupPersistenceFailure(err)
	}
}

func (m *setupManager) logSetupPersistenceFailure(err error) {
	if m.logger != nil {
		m.logger.Warn("setup result could not be persisted", "error", err)
	}
}

func terminalSetupStatus(status string) bool {
	return status == setupJobComplete || status == setupJobFailed || status == setupJobCancelled
}

func sanitizePersistedSetupJob(job setupJob) setupJob {
	job.ID = sanitizeSetupText(job.ID)
	job.Kind = sanitizeSetupText(job.Kind)
	job.Module = sanitizeSetupText(job.Module)
	job.Device = sanitizeSetupText(job.Device)
	job.Status = sanitizeSetupText(job.Status)
	job.Message = sanitizeSetupText(job.Message)
	job.StartedAt = sanitizeSetupText(job.StartedAt)
	job.UpdatedAt = sanitizeSetupText(job.UpdatedAt)
	job.Output = ""
	if len(job.Steps) > 8 {
		job.Steps = job.Steps[:8]
	}
	for index := range job.Steps {
		job.Steps[index].ID = sanitizeSetupText(job.Steps[index].ID)
		job.Steps[index].Label = sanitizeSetupText(job.Steps[index].Label)
		job.Steps[index].Status = sanitizeSetupText(job.Steps[index].Status)
		job.Steps[index].Message = sanitizeSetupText(job.Steps[index].Message)
	}
	return job
}

func sanitizeSetupText(value string) string {
	value = strings.ToValidUTF8(value, "�")
	value = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	if utf8.RuneCountInString(value) <= setupResultTextLimit {
		return value
	}
	runes := []rune(value)
	return string(runes[:setupResultTextLimit])
}
