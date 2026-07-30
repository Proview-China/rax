package invocationmaterialv2_test

import (
	"database/sql"
	"testing"

	modelsqlite "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/storage/sqlite"
)

func TestGovernedModelTurnV4IndexXInfoRequiresLiteralBinaryAndClosedKeys(t *testing.T) {
	valid := []modelsqlite.IndexXInfoConformanceRowV4{
		{
			Sequence:  0,
			CID:       0,
			Column:    sql.NullString{String: "turn_id", Valid: true},
			Collation: "BINARY",
			Key:       1,
		},
		{
			Sequence:  1,
			CID:       -1,
			Collation: "BINARY",
			Key:       0,
		},
	}
	cases := map[string]func([]modelsqlite.IndexXInfoConformanceRowV4){
		"key two": func(rows []modelsqlite.IndexXInfoConformanceRowV4) {
			rows[0].Key = 2
		},
		"key NOCASE": func(rows []modelsqlite.IndexXInfoConformanceRowV4) {
			rows[0].Collation = "NOCASE"
		},
		"key lowercase binary": func(rows []modelsqlite.IndexXInfoConformanceRowV4) {
			rows[0].Collation = "binary"
		},
		"aux NOCASE": func(rows []modelsqlite.IndexXInfoConformanceRowV4) {
			rows[1].Collation = "NOCASE"
		},
		"aux lowercase binary": func(rows []modelsqlite.IndexXInfoConformanceRowV4) {
			rows[1].Collation = "binary"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			rows := append([]modelsqlite.IndexXInfoConformanceRowV4(nil), valid...)
			mutate(rows)
			if _, err := modelsqlite.ValidateIndexXInfoRowsForConformanceV4(rows); err == nil {
				t.Fatal("invalid V4 physical index rows were accepted")
			}
		})
	}
	if columns, err := modelsqlite.ValidateIndexXInfoRowsForConformanceV4(valid); err != nil ||
		len(columns) != 1 || columns[0] != "turn_id" {
		t.Fatalf("valid physical index rejected: columns=%v err=%v", columns, err)
	}
}
