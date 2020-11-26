package brickhouse

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

var reField = regexp.MustCompile("[^0-9A-Za-z_]")
var reValue = regexp.MustCompile("['\r\n\t]")

func createFieldset(fields []string) string {
	cleaned := make([]string, len(fields))
	for i, f := range fields {
		cleaned[i] = fmt.Sprintf("%s VARCHAR", reField.ReplaceAllString(f, ""))
	}

	return strings.Join(cleaned, ",")
}

// Table is a pseudo-enumerator for indicating which table to write/read to/from.
type Table string

// Live, Archive, Changes ...
const (
	Live    Table = "live"
	Archive Table = "archive"
	Changes Table = "changes"
)

// Ensure the given store exists on the database and is accessible.
func Ensure(db *sqlx.DB, label string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	createSchema := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", label)
	if _, err := tx.Exec(createSchema); err != nil {
		return err
	}

	createChangesTable := "CREATE TABLE IF NOT EXISTS %s.%s (%s)"
	fieldset := createFieldset([]string{"brickhouse_id", "keychain", "timestamp", "operation", "old", "new"})
	if _, err := tx.Exec(fmt.Sprintf(createChangesTable, label, Changes, fieldset)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

// Read from the indicated table in the labeled store.
func Read(db *sqlx.DB, label string, table Table) (map[string]interface{}, error) {
	if err := Ensure(db, label); err != nil {
		return nil, err
	}

	rows, err := db.Queryx(fmt.Sprintf("SELECT * FROM %s.%s", label, table))
	if err != nil {
		if _, is := err.(*pq.Error); is && err.(*pq.Error).Code == "42P01" {
			return map[string]interface{}{}, nil
		}

		return nil, err
	}

	// Scan rows into maps, extract record ID, add to data map
	data := map[string]interface{}{}
	for rows.Next() {
		record := map[string]interface{}{}
		if err := rows.MapScan(record); err != nil {
			return nil, err
		}

		if id, in := record["brickhouse_id"]; in {
			delete(record, "brickhouse_id")
			data[id.(string)] = record
		}
	}

	return data, nil
}

// Write to the indicated table in the labeled store.
func Write(db *sqlx.DB, label string, table Table, data map[string]interface{}, drop bool) error {
	if err := Ensure(db, label); err != nil {
		return err
	}

	if drop {
		_, err := db.Exec(fmt.Sprintf("DROP TABLE %s.%s", label, table))
		if _, is := err.(*pq.Error); is && err.(*pq.Error).Code != "42P01" && err != nil {
			return err
		}
	}

	if table != Changes {
		fields := make([]string, 0)
		i := 0
		for _, v := range data {
			if i > 0 {
				break
			}
			for k := range v.(map[string]interface{}) {
				fields = append(fields, k)
			}
			i++
		}
		fieldset := createFieldset(fields)
		if _, err := db.Exec(
			fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s.%s (brickhouse_id VARCHAR,%s)", label, table, fieldset),
		); err != nil {
			return err
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for id, record := range data {
		fields := make([]string, 0)
		values := make([]string, 0)

		for k, v := range record.(map[string]interface{}) {
			fields = append(fields, reField.ReplaceAllString(k, ""))
			values = append(values, fmt.Sprintf("'%s'", reValue.ReplaceAllString(v.(string), "")))
		}

		fieldset := strings.Join(fields, ",")
		valueset := strings.Join(values, ",")

		if _, err := tx.Exec(
			fmt.Sprintf(
				"INSERT INTO %s.%s (brickhouse_id,%s) VALUES ('%s',%s)",
				label, table, fieldset, id, valueset,
			),
		); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}
