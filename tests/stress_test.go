package tests

import (
	"encoding/csv"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/sr-revops/bricks"
)

func TestStressUpdate(t *testing.T) {
	f, err := os.Open("temp.large.csv")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	raw, err := reader.ReadAll()

	data := map[string]interface{}{}
	for _, row := range raw {
		record := map[string]interface{}{}
		for i, cell := range row {
			record[raw[0][i]] = cell
		}
		data[row[18]] = record
	}

	db, err := sqlx.Open(
		"postgres",
		"postgresql://postgres:dev@localhost:5432/bricks?sslmode=disable")
	if err != nil {
		t.Error(err)
	}
	defer db.Close()

	if err := bricks.Update(db, "thrutest", data); err != nil {
		t.Error(err)
	}
	target := time.Now()

	for i := 1; i < len(data)/2; i++ {
		delete(data, raw[i][18])
	}

	if err := bricks.Update(db, "thrutest", data); err != nil {
		t.Error(err)
	}

	if out, err := bricks.Restore(db, "thrutest", target); err != nil {
		t.Error(err)
	} else if len(out) == len(data) {
		t.Error(errors.New("deletions didn't work"))
	}
}