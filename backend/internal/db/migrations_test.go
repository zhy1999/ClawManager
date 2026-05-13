package db

import (
	"reflect"
	"testing"
)

func TestSplitSQLStatements(t *testing.T) {
	input := `
-- create table comment
CREATE TABLE demo (
  id INT PRIMARY KEY,
  note VARCHAR(255) DEFAULT 'hello;world'
);

/* block comment */
INSERT INTO demo (id, note) VALUES (1, 'value');
UPDATE demo SET note = "a;quoted" WHERE id = 1;
`

	got := splitSQLStatements(input)
	want := []string{
		"CREATE TABLE demo (\n  id INT PRIMARY KEY,\n  note VARCHAR(255) DEFAULT 'hello;world'\n)",
		"INSERT INTO demo (id, note) VALUES (1, 'value')",
		`UPDATE demo SET note = "a;quoted" WHERE id = 1`,
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected statements:\nwant: %#v\ngot: %#v", want, got)
	}
}
