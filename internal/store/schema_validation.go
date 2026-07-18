package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type schemaColumn struct {
	name     string
	typeName string
}

type schemaTable struct {
	name       string
	columns    []schemaColumn
	primaryKey []string
}

type schemaIndexColumn struct {
	name       string
	descending bool
}

type schemaIndex struct {
	table   string
	name    string
	unique  bool
	columns []schemaIndexColumn
}

type schemaForeignKey struct {
	table        string
	column       string
	parentTable  string
	parentColumn string
	onDelete     string
}

type actualSchemaColumn struct {
	typeName   string
	notNull    bool
	primaryKey int
}

var requiredSchemaTables = []schemaTable{
	{name: "settings", columns: columns("id:TEXT", "document:TEXT", "updated_at:TEXT"), primaryKey: []string{"id"}},
	{name: "app_kv", columns: columns("key:TEXT", "value:TEXT", "updated_at:TEXT"), primaryKey: []string{"key"}},
	{name: "memories", columns: columns("id:TEXT", "text:TEXT", "enabled:INTEGER", "created_at:TEXT"), primaryKey: []string{"id"}},
	{name: "prompt_sets", columns: columns("id:TEXT", "name:TEXT", "system:TEXT", "created_at:TEXT"), primaryKey: []string{"id"}},
	{name: "legacy_imports", columns: columns(
		"domain:TEXT", "source_path:TEXT", "archived_path:TEXT", "status:TEXT", "message:TEXT", "imported_at:TEXT",
	), primaryKey: []string{"domain"}},
	{name: "messages", columns: columns(
		"seq:INTEGER", "role:TEXT", "content:TEXT", "client_id:TEXT", "created_at:TEXT",
	), primaryKey: []string{"seq"}},
	{name: "client_cursors", columns: columns("client_id:TEXT", "last_seq:INTEGER", "updated_at:TEXT"), primaryKey: []string{"client_id"}},
	{name: "patterns", columns: columns(
		"id:TEXT", "name:TEXT", "description:TEXT", "origin:TEXT", "kind:TEXT", "enabled:INTEGER",
		"weight:REAL", "cycle_ms:INTEGER", "points_json:TEXT", "tags_json:TEXT", "created_at:TEXT", "updated_at:TEXT",
	), primaryKey: []string{"id"}},
	{name: "programs", columns: columns(
		"id:TEXT", "name:TEXT", "origin:TEXT", "duration_ms:INTEGER", "points_json:TEXT", "created_at:TEXT", "updated_at:TEXT",
	), primaryKey: []string{"id"}},
	{name: "pattern_feedback", columns: columns(
		"id:INTEGER", "pattern_id:TEXT", "rating:INTEGER", "weight_before:REAL", "weight_after:REAL",
		"enabled_before:INTEGER", "enabled_after:INTEGER", "reverted:INTEGER", "created_at:TEXT", "reverted_at:TEXT",
	), primaryKey: []string{"id"}},
	{name: "llm_models", columns: columns(
		"id:TEXT", "display_name:TEXT", "provider:TEXT", "source:TEXT", "source_name:TEXT", "format:TEXT",
		"family:TEXT", "parameter_size:TEXT", "quantization:TEXT", "size_bytes:INTEGER", "sha256:TEXT",
		"model_path:TEXT", "license:TEXT", "imported_at:TEXT", "updated_at:TEXT",
	), primaryKey: []string{"id"}},
	{name: "settings_recoveries", columns: columns("id:INTEGER", "document:TEXT", "reason:TEXT", "recovered_at:TEXT"), primaryKey: []string{"id"}},
}

var requiredSchemaIndexes = []schemaIndex{
	{table: "patterns", name: "patterns_enabled_weight", columns: indexColumns("enabled", "-weight", "name")},
	{table: "pattern_feedback", name: "pattern_feedback_pattern_created", columns: indexColumns("pattern_id", "-id")},
	{table: "llm_models", name: "llm_models_sha256", unique: true, columns: indexColumns("sha256")},
	{table: "llm_models", name: "llm_models_path", unique: true, columns: indexColumns("model_path")},
	{table: "settings_recoveries", name: "settings_recoveries_recovered_at", columns: indexColumns("-recovered_at", "-id")},
}

var requiredSchemaForeignKeys = []schemaForeignKey{
	{
		table:        "pattern_feedback",
		column:       "pattern_id",
		parentTable:  "patterns",
		parentColumn: "id",
		onDelete:     "CASCADE",
	},
}

func (db *DB) validateSchema(ctx context.Context) error {
	checks := []func(context.Context) error{
		db.validateSchemaVersion,
		db.validateForeignKeyEnforcement,
		db.validateSchemaTables,
		db.validateSchemaIndexes,
		db.validateSchemaForeignKeys,
		db.validateForeignKeyRows,
	}
	for _, check := range checks {
		if err := check(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) validateSchemaVersion(ctx context.Context) error {
	version, err := db.schemaVersion(ctx)
	if err != nil {
		return err
	}
	if version != CurrentSchemaVersion {
		return fmt.Errorf("%w: database is v%d after migration, want v%d", ErrInvalidSchema, version, CurrentSchemaVersion)
	}
	return nil
}

func (db *DB) validateForeignKeyEnforcement(ctx context.Context) error {
	var foreignKeys int
	if err := db.sql.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return fmt.Errorf("validate SQLite foreign keys: %w", err)
	}
	if foreignKeys != 1 {
		return fmt.Errorf("%w: foreign key enforcement is disabled", ErrInvalidSchema)
	}
	return nil
}

func (db *DB) validateSchemaTables(ctx context.Context) error {
	for _, table := range requiredSchemaTables {
		actual, err := schemaColumns(ctx, db.sql, table.name)
		if err != nil {
			return err
		}
		if len(actual) == 0 {
			return fmt.Errorf("%w: required table %q is missing", ErrInvalidSchema, table.name)
		}
		for _, column := range table.columns {
			actualColumn, ok := actual[strings.ToLower(column.name)]
			if !ok {
				return fmt.Errorf("%w: required column %s.%s is missing", ErrInvalidSchema, table.name, column.name)
			}
			if !strings.EqualFold(actualColumn.typeName, column.typeName) {
				return fmt.Errorf(
					"%w: column %s.%s has type %q, want %q",
					ErrInvalidSchema,
					table.name,
					column.name,
					actualColumn.typeName,
					column.typeName,
				)
			}
			if actualColumn.primaryKey == 0 && !actualColumn.notNull {
				return fmt.Errorf("%w: required column %s.%s permits NULL", ErrInvalidSchema, table.name, column.name)
			}
		}
		actualPrimaryKey := make([]string, len(table.primaryKey))
		for name, column := range actual {
			if column.primaryKey == 0 {
				continue
			}
			if column.primaryKey > len(actualPrimaryKey) {
				return fmt.Errorf("%w: table %q has an unexpected primary key", ErrInvalidSchema, table.name)
			}
			actualPrimaryKey[column.primaryKey-1] = name
		}
		for index, expected := range table.primaryKey {
			if !strings.EqualFold(actualPrimaryKey[index], expected) {
				return fmt.Errorf(
					"%w: table %q primary key column %d is %q, want %q",
					ErrInvalidSchema,
					table.name,
					index+1,
					actualPrimaryKey[index],
					expected,
				)
			}
		}
	}
	return nil
}

func (db *DB) validateSchemaIndexes(ctx context.Context) error {
	for _, index := range requiredSchemaIndexes {
		actual, exists, err := schemaIndexDefinition(ctx, db.sql, index.table, index.name)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: required index %q is missing", ErrInvalidSchema, index.name)
		}
		if actual.unique != index.unique {
			return fmt.Errorf("%w: index %q uniqueness does not match the schema", ErrInvalidSchema, index.name)
		}
		if !sameIndexColumns(actual.columns, index.columns) {
			return fmt.Errorf("%w: index %q columns do not match the schema", ErrInvalidSchema, index.name)
		}
	}
	return nil
}

func (db *DB) validateSchemaForeignKeys(ctx context.Context) error {
	for _, foreignKey := range requiredSchemaForeignKeys {
		exists, err := schemaForeignKeyExists(ctx, db.sql, foreignKey)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf(
				"%w: required foreign key %s.%s -> %s.%s is missing",
				ErrInvalidSchema,
				foreignKey.table,
				foreignKey.column,
				foreignKey.parentTable,
				foreignKey.parentColumn,
			)
		}
	}
	return nil
}

func (db *DB) validateForeignKeyRows(ctx context.Context) error {
	rows, err := db.sql.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("validate SQLite foreign key rows: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		var table, parent string
		var rowID sql.NullInt64
		var foreignKeyID int
		if err := rows.Scan(&table, &rowID, &parent, &foreignKeyID); err != nil {
			return fmt.Errorf("scan SQLite foreign key violation: %w", err)
		}
		return fmt.Errorf(
			"%w: foreign key violation in table %q row %d referencing %q (constraint %d)",
			ErrInvalidSchema,
			table,
			rowID.Int64,
			parent,
			foreignKeyID,
		)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read SQLite foreign key validation: %w", err)
	}
	return nil
}

func columns(specifications ...string) []schemaColumn {
	result := make([]schemaColumn, 0, len(specifications))
	for _, specification := range specifications {
		name, typeName, _ := strings.Cut(specification, ":")
		result = append(result, schemaColumn{name: name, typeName: typeName})
	}
	return result
}

func indexColumns(specifications ...string) []schemaIndexColumn {
	result := make([]schemaIndexColumn, 0, len(specifications))
	for _, specification := range specifications {
		descending := strings.HasPrefix(specification, "-")
		result = append(result, schemaIndexColumn{
			name:       strings.TrimPrefix(specification, "-"),
			descending: descending,
		})
	}
	return result
}

func schemaColumns(ctx context.Context, db *sql.DB, table string) (map[string]actualSchemaColumn, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table)) // #nosec G201 -- internal constants only.
	if err != nil {
		return nil, fmt.Errorf("inspect %s table: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	actual := make(map[string]actualSchemaColumn)
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, typeName string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typeName, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, fmt.Errorf("scan %s table metadata: %w", table, err)
		}
		actual[strings.ToLower(name)] = actualSchemaColumn{
			typeName:   typeName,
			notNull:    notNull != 0,
			primaryKey: primaryKey,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read %s table metadata: %w", table, err)
	}
	return actual, nil
}

func schemaIndexDefinition(ctx context.Context, db *sql.DB, table, target string) (schemaIndex, bool, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_list(%s)", table)) // #nosec G201 -- internal constants only.
	if err != nil {
		return schemaIndex{}, false, fmt.Errorf("inspect %s indexes: %w", table, err)
	}
	found := false
	unique := false
	for rows.Next() {
		var sequence, uniqueValue, partial int
		var name, origin string
		if err := rows.Scan(&sequence, &name, &uniqueValue, &origin, &partial); err != nil {
			_ = rows.Close()
			return schemaIndex{}, false, fmt.Errorf("scan %s index metadata: %w", table, err)
		}
		if strings.EqualFold(name, target) {
			found = true
			unique = uniqueValue != 0
			break
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return schemaIndex{}, false, fmt.Errorf("read %s index metadata: %w", table, err)
	}
	if err := rows.Close(); err != nil {
		return schemaIndex{}, false, fmt.Errorf("close %s index metadata: %w", table, err)
	}
	if !found {
		return schemaIndex{}, false, nil
	}

	definition := schemaIndex{table: table, name: target, unique: unique}
	columnRows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_xinfo(%s)", target)) // #nosec G201 -- internal constants only.
	if err != nil {
		return schemaIndex{}, false, fmt.Errorf("inspect index %s columns: %w", target, err)
	}
	defer func() { _ = columnRows.Close() }()
	for columnRows.Next() {
		var sequence, columnID, descending, key int
		var name, collation sql.NullString
		if err := columnRows.Scan(&sequence, &columnID, &name, &descending, &collation, &key); err != nil {
			return schemaIndex{}, false, fmt.Errorf("scan index %s columns: %w", target, err)
		}
		if key == 0 {
			continue
		}
		if !name.Valid {
			return schemaIndex{}, false, fmt.Errorf("%w: index %q contains an expression", ErrInvalidSchema, target)
		}
		definition.columns = append(definition.columns, schemaIndexColumn{
			name:       name.String,
			descending: descending != 0,
		})
	}
	if err := columnRows.Err(); err != nil {
		return schemaIndex{}, false, fmt.Errorf("read index %s columns: %w", target, err)
	}
	return definition, true, nil
}

func sameIndexColumns(left, right []schemaIndexColumn) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !strings.EqualFold(left[index].name, right[index].name) ||
			left[index].descending != right[index].descending {
			return false
		}
	}
	return true
}

func schemaForeignKeyExists(ctx context.Context, db *sql.DB, expected schemaForeignKey) (bool, error) {
	rows, err := db.QueryContext(
		ctx,
		fmt.Sprintf("PRAGMA foreign_key_list(%s)", expected.table), // #nosec G201 -- internal constants only.
	)
	if err != nil {
		return false, fmt.Errorf("inspect %s foreign keys: %w", expected.table, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, sequence int
		var parentTable, column, parentColumn, onUpdate, onDelete, match string
		if err := rows.Scan(
			&id,
			&sequence,
			&parentTable,
			&column,
			&parentColumn,
			&onUpdate,
			&onDelete,
			&match,
		); err != nil {
			return false, fmt.Errorf("scan %s foreign keys: %w", expected.table, err)
		}
		if strings.EqualFold(column, expected.column) &&
			strings.EqualFold(parentTable, expected.parentTable) &&
			strings.EqualFold(parentColumn, expected.parentColumn) &&
			strings.EqualFold(onDelete, expected.onDelete) {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("read %s foreign keys: %w", expected.table, err)
	}
	return false, nil
}
