package patterns

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/mapledaemon/MagicHandy/internal/motion"
	dbstore "github.com/mapledaemon/MagicHandy/internal/store"
)

// Library owns durable pattern/program rows in the app datastore.
type Library struct {
	db     *dbstore.DB
	ownsDB bool
}

const patternByIDQuery = `
	SELECT id, name, description, origin, kind, enabled, weight, cycle_ms,
	       points_json, tags_json, created_at, updated_at
	FROM patterns WHERE id = ?
`

// Open opens the library and seeds code-generated built-ins without replacing
// user enablement or feedback-derived weights.
func Open(dataDir string) (*Library, error) {
	database, err := dbstore.Open(dataDir)
	if err != nil {
		return nil, fmt.Errorf("open pattern library: %w", err)
	}
	library, err := openWithDatabase(database, true)
	if err != nil {
		_ = database.Close()
	}
	return library, err
}

// OpenWithDatabase borrows the process-owned datastore for pattern content.
func OpenWithDatabase(database *dbstore.DB) (*Library, error) {
	if database == nil {
		return nil, errors.New("pattern datastore is required")
	}
	return openWithDatabase(database, false)
}

func openWithDatabase(database *dbstore.DB, ownsDB bool) (*Library, error) {
	library := &Library{db: database, ownsDB: ownsDB}
	if err := library.seedBuiltins(context.Background()); err != nil {
		return nil, err
	}
	return library, nil
}

// Close releases the library database handle.
func (l *Library) Close() error {
	if !l.ownsDB {
		return nil
	}
	return l.db.Close()
}

// Snapshot returns all content plus recent reversible feedback.
func (l *Library) Snapshot() (Snapshot, error) {
	patterns, err := l.ListPatterns()
	if err != nil {
		return Snapshot{}, err
	}
	programs, err := l.ListPrograms()
	if err != nil {
		return Snapshot{}, err
	}
	feedback, err := l.FeedbackHistory(30)
	if err != nil {
		return Snapshot{}, err
	}
	autoDisable, err := l.AutoDisable()
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		Patterns:    patterns,
		Programs:    programs,
		Feedback:    feedback,
		AutoDisable: autoDisable,
	}, nil
}

// Summary returns indexed counts for the regular app-state poll.
func (l *Library) Summary() (Summary, error) {
	ctx := context.Background()
	summary := Summary{Available: true}
	if err := l.db.SQL().QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(enabled), 0) FROM patterns
	`).Scan(&summary.PatternCount, &summary.EnabledPatternCount); err != nil {
		return Summary{}, err
	}
	if err := l.db.SQL().QueryRowContext(ctx, `SELECT COUNT(*) FROM programs`).Scan(&summary.ProgramCount); err != nil {
		return Summary{}, err
	}
	autoDisable, err := l.AutoDisable()
	if err != nil {
		return Summary{}, err
	}
	summary.AutoDisable = autoDisable
	return summary, nil
}

// ListPatterns returns built-ins first, then user content by name.
func (l *Library) ListPatterns() ([]Pattern, error) {
	rows, err := l.db.SQL().QueryContext(context.Background(), `
		SELECT id, name, description, origin, kind, enabled, weight, cycle_ms,
		       points_json, tags_json, created_at, updated_at
		FROM patterns
		ORDER BY CASE origin WHEN 'builtin' THEN 0 ELSE 1 END, name, id
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	patterns := make([]Pattern, 0)
	for rows.Next() {
		pattern, scanErr := scanPattern(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		patterns = append(patterns, pattern)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return patterns, nil
}

// Pattern returns one row by ID.
func (l *Library) Pattern(id string) (Pattern, error) {
	pattern, err := queryPattern(context.Background(), l.db.SQL(), id)
	if errors.Is(err, sql.ErrNoRows) {
		return Pattern{}, ErrPatternNotFound
	}
	return pattern, err
}

// ResolveEnabled returns engine content only when the entry remains enabled.
func (l *Library) ResolveEnabled(id string) (motion.PatternDefinition, bool, error) {
	pattern, err := l.Pattern(id)
	if errors.Is(err, ErrPatternNotFound) || (err == nil && !pattern.Enabled) {
		return motion.PatternDefinition{}, false, nil
	}
	if err != nil {
		return motion.PatternDefinition{}, false, err
	}
	definition, err := motion.NormalizePatternDefinition(pattern.Definition())
	if err != nil {
		return motion.PatternDefinition{}, false, err
	}
	return definition, true, nil
}

// EnabledChoices returns prompt-safe metadata, weighted preference first.
func (l *Library) EnabledChoices() ([]CurationChoice, error) {
	rows, err := l.db.SQL().QueryContext(context.Background(), `
		SELECT id, name, description, tags_json, weight
		FROM patterns WHERE enabled = 1
		ORDER BY weight DESC, name, id
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	choices := make([]CurationChoice, 0)
	for rows.Next() {
		var choice CurationChoice
		var tagsJSON string
		if err := rows.Scan(&choice.ID, &choice.Name, &choice.Description, &tagsJSON, &choice.Weight); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(tagsJSON), &choice.Tags); err != nil {
			return nil, err
		}
		choices = append(choices, choice)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return choices, nil
}

// CreatePattern validates, simplifies, and stores one shareable user pattern.
func (l *Library) CreatePattern(input PatternInput) (Pattern, error) {
	if err := l.ensurePatternCapacity(); err != nil {
		return Pattern{}, err
	}
	name, description, err := validateMetadata(input.Name, input.Description)
	if err != nil {
		return Pattern{}, err
	}
	preview, err := PreviewPattern(input)
	if err != nil {
		return Pattern{}, fmt.Errorf("%w: %v", ErrInvalidContent, err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	pattern := Pattern{
		ID: userContentID("pattern", name), Name: name, Description: description,
		Origin: OriginUser, Kind: normalizeKind(input.Kind), Enabled: true, Weight: 1,
		CycleMillis: preview.CycleMillis, Points: preview.Points, Tags: normalizeStringTags(input.Tags),
		CreatedAt: now, UpdatedAt: now,
	}
	pattern, err = withPatternPreview(pattern)
	if err != nil {
		return Pattern{}, fmt.Errorf("%w: %v", ErrInvalidContent, err)
	}
	if err := l.insertPattern(pattern); err != nil {
		return Pattern{}, err
	}
	return pattern, nil
}

// UpdatePattern applies visible row controls and editable user content.
func (l *Library) UpdatePattern(id string, patch PatternPatch) (Pattern, error) {
	ctx := context.Background()
	var next Pattern
	if err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := queryPattern(ctx, tx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPatternNotFound
		}
		if err != nil {
			return err
		}
		if current.Origin == OriginBuiltin && patchChangesContent(patch) {
			return ErrBuiltinPattern
		}
		next, err = applyPatternPatch(current, patch)
		if err != nil {
			return err
		}
		next, err = withPatternPreview(next)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidContent, err)
		}
		next.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		pointsJSON, tagsJSON, err := encodePatternData(next)
		if err != nil {
			return err
		}
		result, updateErr := tx.ExecContext(ctx, `
			UPDATE patterns SET name = ?, description = ?, kind = ?, enabled = ?,
				weight = ?, cycle_ms = ?, points_json = ?, tags_json = ?, updated_at = ?
			WHERE id = ?
		`, next.Name, next.Description, next.Kind, boolInt(next.Enabled), next.Weight,
			next.CycleMillis, pointsJSON, tagsJSON, next.UpdatedAt, next.ID)
		if updateErr != nil {
			return updateErr
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrPatternNotFound
		}
		return nil
	}); err != nil {
		return Pattern{}, err
	}
	return next, nil
}

// DeletePattern removes user/generated content; built-ins remain available.
func (l *Library) DeletePattern(id string) error {
	pattern, err := l.Pattern(id)
	if err != nil {
		return err
	}
	if pattern.Origin == OriginBuiltin {
		return ErrBuiltinPattern
	}
	ctx := context.Background()
	return l.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, deleteErr := tx.ExecContext(ctx, `DELETE FROM patterns WHERE id = ?`, pattern.ID)
		if deleteErr != nil {
			return deleteErr
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrPatternNotFound
		}
		return nil
	})
}

// ListPrograms returns finite content by name.
func (l *Library) ListPrograms() ([]Program, error) {
	rows, err := l.db.SQL().QueryContext(context.Background(), `
		SELECT id, name, origin, duration_ms, points_json, created_at, updated_at
		FROM programs ORDER BY name, id
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	programs := make([]Program, 0)
	for rows.Next() {
		program, scanErr := scanProgram(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		programs = append(programs, program)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return programs, nil
}

// Program returns one finite entry by ID.
func (l *Library) Program(id string) (Program, error) {
	row := l.db.SQL().QueryRowContext(context.Background(), `
		SELECT id, name, origin, duration_ms, points_json, created_at, updated_at
		FROM programs WHERE id = ?
	`, strings.TrimSpace(id))
	program, err := scanProgram(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Program{}, ErrProgramNotFound
	}
	if err != nil {
		return Program{}, err
	}
	return program, nil
}

// CreateProgram validates and stores finite motion content.
func (l *Library) CreateProgram(name, origin string, points []motion.CurvePoint, duration int64) (Program, error) {
	if err := l.ensureProgramCapacity(); err != nil {
		return Program{}, err
	}
	name, _, err := validateMetadata(name, "")
	if err != nil {
		return Program{}, err
	}
	prepared, err := prepareRawPoints(points, duration)
	if err != nil {
		return Program{}, fmt.Errorf("%w: %v", ErrInvalidContent, err)
	}
	definition, err := motion.NormalizeProgramDefinition(motion.ProgramDefinition{
		ID: userContentID("program", name), Name: name, DurationMillis: duration, Points: prepared,
	})
	if err != nil {
		return Program{}, fmt.Errorf("%w: %v", ErrInvalidContent, err)
	}
	if origin != OriginImported {
		origin = OriginUser
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	program := Program{
		ID: definition.ID, Name: definition.Name, Origin: origin,
		DurationMillis: definition.DurationMillis, Points: definition.Points,
		CreatedAt: now, UpdatedAt: now,
	}
	program, err = withProgramPreview(program)
	if err != nil {
		return Program{}, fmt.Errorf("%w: %v", ErrInvalidContent, err)
	}
	data, err := json.Marshal(program.Points)
	if err != nil {
		return Program{}, err
	}
	ctx := context.Background()
	err = l.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := ensureCapacityTx(ctx, tx, "programs", maxPrograms, "program"); err != nil {
			return err
		}
		_, insertErr := tx.ExecContext(ctx, `
			INSERT INTO programs(id, name, origin, duration_ms, points_json, created_at, updated_at)
			VALUES(?, ?, ?, ?, ?, ?, ?)
		`, program.ID, program.Name, program.Origin, program.DurationMillis, string(data), now, now)
		return insertErr
	})
	if err != nil {
		return Program{}, err
	}
	return program, nil
}

// DeleteProgram removes one finite entry.
func (l *Library) DeleteProgram(id string) error {
	ctx := context.Background()
	return l.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `DELETE FROM programs WHERE id = ?`, strings.TrimSpace(id))
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrProgramNotFound
		}
		return nil
	})
}

func (l *Library) seedBuiltins(ctx context.Context) error {
	return l.db.WithTx(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		for _, definition := range motion.BuiltinPatternDefinitions() {
			points, _ := json.Marshal(definition.Points)
			tags, _ := json.Marshal(definition.Tags)
			_, err := tx.ExecContext(ctx, `
				INSERT INTO patterns(id, name, description, origin, kind, enabled, weight,
					cycle_ms, points_json, tags_json, created_at, updated_at)
				VALUES(?, ?, ?, 'builtin', ?, 1, 1.0, ?, ?, ?, ?, ?)
				ON CONFLICT(id) DO UPDATE SET
					name = excluded.name, description = excluded.description,
					origin = 'builtin', kind = excluded.kind, cycle_ms = excluded.cycle_ms,
					points_json = excluded.points_json, tags_json = excluded.tags_json,
					updated_at = excluded.updated_at
			`, definition.ID, definition.Name, definition.Description, definition.Kind,
				definition.CycleMillis, string(points), string(tags), now, now)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (l *Library) insertPattern(pattern Pattern) error {
	pointsJSON, tagsJSON, err := encodePatternData(pattern)
	if err != nil {
		return err
	}
	ctx := context.Background()
	return l.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := ensureCapacityTx(ctx, tx, "patterns", maxPatterns, "pattern"); err != nil {
			return err
		}
		_, insertErr := tx.ExecContext(ctx, `
			INSERT INTO patterns(id, name, description, origin, kind, enabled, weight,
				cycle_ms, points_json, tags_json, created_at, updated_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, pattern.ID, pattern.Name, pattern.Description, pattern.Origin, pattern.Kind,
			boolInt(pattern.Enabled), pattern.Weight, pattern.CycleMillis, pointsJSON,
			tagsJSON, pattern.CreatedAt, pattern.UpdatedAt)
		return insertErr
	})
}

func applyPatternPatch(current Pattern, patch PatternPatch) (Pattern, error) {
	next := current
	if patch.Name != nil {
		next.Name = *patch.Name
	}
	if patch.Description != nil {
		next.Description = *patch.Description
	}
	if patch.Kind != nil {
		next.Kind = *patch.Kind
	}
	if patch.Enabled != nil {
		next.Enabled = *patch.Enabled
	}
	if patch.Weight != nil {
		if math.IsNaN(*patch.Weight) || math.IsInf(*patch.Weight, 0) {
			return Pattern{}, fmt.Errorf("%w: pattern weight must be finite", ErrInvalidContent)
		}
		next.Weight = mathClamp(*patch.Weight, 0.1, 3)
	}
	if patch.CycleMillis != nil {
		next.CycleMillis = *patch.CycleMillis
	}
	if patch.Points != nil {
		next.Points = append([]motion.CurvePoint(nil), (*patch.Points)...)
	}
	if patch.Tags != nil {
		next.Tags = normalizeStringTags(*patch.Tags)
	}
	name, description, err := validateMetadata(next.Name, next.Description)
	if err != nil {
		return Pattern{}, err
	}
	next.Name, next.Description = name, description
	definition, err := motion.NormalizePatternDefinition(next.Definition())
	if err != nil {
		return Pattern{}, fmt.Errorf("%w: %v", ErrInvalidContent, err)
	}
	next.Kind, next.CycleMillis, next.Points, next.Tags = definition.Kind, definition.CycleMillis, definition.Points, definition.Tags
	return next, nil
}

func scanPattern(scanner interface{ Scan(...any) error }) (Pattern, error) {
	var pattern Pattern
	var enabled int
	var pointsJSON, tagsJSON string
	err := scanner.Scan(&pattern.ID, &pattern.Name, &pattern.Description, &pattern.Origin,
		&pattern.Kind, &enabled, &pattern.Weight, &pattern.CycleMillis, &pointsJSON,
		&tagsJSON, &pattern.CreatedAt, &pattern.UpdatedAt)
	if err != nil {
		return Pattern{}, err
	}
	pattern.Enabled = enabled != 0
	if err := json.Unmarshal([]byte(pointsJSON), &pattern.Points); err != nil {
		return Pattern{}, err
	}
	if err := json.Unmarshal([]byte(tagsJSON), &pattern.Tags); err != nil {
		return Pattern{}, err
	}
	return withPatternPreview(pattern)
}

func withPatternPreview(pattern Pattern) (Pattern, error) {
	definition, err := motion.NormalizePatternDefinition(pattern.Definition())
	if err != nil {
		return Pattern{}, err
	}
	curve, err := motion.NewCurve(definition.Points, definition.CycleMillis, true)
	if err != nil {
		return Pattern{}, err
	}
	pattern.PreviewSamples = curve.Preview(max(int64(25), definition.CycleMillis/72))
	return pattern, nil
}

func scanProgram(scanner interface{ Scan(...any) error }) (Program, error) {
	var program Program
	var pointsJSON string
	err := scanner.Scan(&program.ID, &program.Name, &program.Origin, &program.DurationMillis,
		&pointsJSON, &program.CreatedAt, &program.UpdatedAt)
	if err != nil {
		return Program{}, err
	}
	if err := json.Unmarshal([]byte(pointsJSON), &program.Points); err != nil {
		return Program{}, err
	}
	return withProgramPreview(program)
}

func withProgramPreview(program Program) (Program, error) {
	definition, err := motion.NormalizeProgramDefinition(program.Definition())
	if err != nil {
		return Program{}, err
	}
	curve, err := motion.NewCurve(definition.Points, definition.DurationMillis, false)
	if err != nil {
		return Program{}, err
	}
	program.PreviewSamples = curve.Preview(max(int64(25), definition.DurationMillis/72))
	return program, nil
}

func encodePatternData(pattern Pattern) (string, string, error) {
	points, err := json.Marshal(pattern.Points)
	if err != nil {
		return "", "", err
	}
	tags, err := json.Marshal(pattern.Tags)
	return string(points), string(tags), err
}

func validateMetadata(name, description string) (string, string, error) {
	name, description = strings.TrimSpace(name), strings.TrimSpace(description)
	if name == "" {
		return "", "", fmt.Errorf("%w: content name is required", ErrInvalidContent)
	}
	if len([]rune(name)) > maxContentNameChars {
		return "", "", fmt.Errorf("%w: content name must be at most %d characters", ErrInvalidContent, maxContentNameChars)
	}
	if len([]rune(description)) > maxDescriptionChars {
		return "", "", fmt.Errorf("%w: description must be at most %d characters", ErrInvalidContent, maxDescriptionChars)
	}
	return name, description, nil
}

func (l *Library) ensurePatternCapacity() error {
	var count int
	if err := l.db.SQL().QueryRow("SELECT COUNT(*) FROM patterns").Scan(&count); err != nil {
		return err
	}
	if count >= maxPatterns {
		return fmt.Errorf("%w: pattern limit reached (%d)", ErrLibraryLimit, maxPatterns)
	}
	return nil
}

func (l *Library) ensureProgramCapacity() error {
	var count int
	if err := l.db.SQL().QueryRow("SELECT COUNT(*) FROM programs").Scan(&count); err != nil {
		return err
	}
	if count >= maxPrograms {
		return fmt.Errorf("%w: program limit reached (%d)", ErrLibraryLimit, maxPrograms)
	}
	return nil
}

type patternQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func queryPattern(ctx context.Context, queryer patternQueryer, id string) (Pattern, error) {
	return scanPattern(queryer.QueryRowContext(ctx, patternByIDQuery, strings.TrimSpace(id)))
}

func ensureCapacityTx(ctx context.Context, tx *sql.Tx, table string, limit int, label string) error {
	var query string
	switch table {
	case "patterns":
		query = "SELECT COUNT(*) FROM patterns"
	case "programs":
		query = "SELECT COUNT(*) FROM programs"
	default:
		return fmt.Errorf("unsupported capacity table %q", table)
	}
	var count int
	if err := tx.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return err
	}
	if count >= limit {
		return fmt.Errorf("%w: %s limit reached (%d)", ErrLibraryLimit, label, limit)
	}
	return nil
}

func userContentID(prefix, name string) string {
	slug := strings.Trim(strings.Map(func(character rune) rune {
		switch {
		case unicode.IsLetter(character), unicode.IsDigit(character):
			return unicode.ToLower(character)
		case character == ' ', character == '-', character == '_':
			return '-'
		default:
			return -1
		}
	}, name), "-")
	runes := []rune(slug)
	if len(runes) > 32 {
		slug = string(runes[:32])
	}
	if slug == "" {
		slug = "content"
	}
	buffer := make([]byte, 4)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%s-%s-%d", prefix, slug, time.Now().UnixNano())
	}
	return prefix + "-" + slug + "-" + hex.EncodeToString(buffer)
}

func patchChangesContent(patch PatternPatch) bool {
	return patch.Name != nil || patch.Description != nil || patch.Kind != nil ||
		patch.CycleMillis != nil || patch.Points != nil || patch.Tags != nil
}

func normalizeKind(kind string) string {
	if strings.EqualFold(strings.TrimSpace(kind), motion.PatternKindBurst) {
		return motion.PatternKindBurst
	}
	return motion.PatternKindRoutine
}

func normalizeStringTags(tags []string) []string {
	definition, err := motion.NormalizePatternDefinition(motion.PatternDefinition{
		ID: "tags", Name: "Tags", Kind: motion.PatternKindRoutine,
		CycleMillis: motion.RoutineCycleFloorMillis,
		Points:      []motion.CurvePoint{{TimeMillis: 0}, {TimeMillis: motion.RoutineCycleFloorMillis / 2, PositionPercent: 100}, {TimeMillis: motion.RoutineCycleFloorMillis}},
		Tags:        tags,
	})
	if err != nil {
		return nil
	}
	return definition.Tags
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func mathClamp(value, minimum, maximum float64) float64 {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
