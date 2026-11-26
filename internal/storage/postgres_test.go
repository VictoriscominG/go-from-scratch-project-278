package storage

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsUniqueViolation(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "23505"}
	if !isUniqueViolation(pgErr) {
		t.Fatal("23505 must be recognized as unique violation")
	}

	other := &pgconn.PgError{Code: "42P01"}
	if isUniqueViolation(other) {
		t.Fatal("42P01 must not be unique violation")
	}

	if isUniqueViolation(errors.New("plain error")) {
		t.Fatal("plain error must not be unique violation")
	}
}
