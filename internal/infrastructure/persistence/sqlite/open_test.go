package sqlite

import (
	"path/filepath"
	"testing"
)

func TestOpenReusesAppliedOwnedMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.db")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := Open(path)
	if err != nil {
		t.Fatalf("reopen Delivery database: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}
