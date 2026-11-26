package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/yeboahd24/paystack-billing/migrations"
)

// Databases from before 002 could hold several 'active' rows per user; the
// migration keeps the newest and enforces one current row from then on.
func TestMigration002KeepsNewestCurrentRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpTo(conn, ".", 1); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (id, email, name, tier) VALUES (1, 'a@x', 'A', 'pro'), (2, 'b@x', 'B', 'pro')`,
		`INSERT INTO subscriptions (user_id, plan, provider_reference, status, current_period_end) VALUES
			(1, 'pro', 'r1', 'active', '2026-01-01 00:00:00'),
			(1, 'pro', 'r2', 'active', '2026-02-01 00:00:00'),
			(1, 'pro', 'r3', 'cancelled', '2026-03-01 00:00:00'),
			(2, 'pro', 'r4', 'cancelled', '2025-06-01 00:00:00')`,
	} {
		if _, err := conn.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	conn.Close()

	d, err := Open(path) // runs 002
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	got := map[string]string{}
	rows, err := d.Query(`SELECT provider_reference, status FROM subscriptions`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var ref, st string
		if err := rows.Scan(&ref, &st); err != nil {
			t.Fatal(err)
		}
		got[ref] = st
	}
	rows.Close()
	want := map[string]string{"r1": "superseded", "r2": "superseded", "r3": "cancelled", "r4": "cancelled"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %s, want %s", k, got[k], v)
		}
	}
	// A second current row for user 1 is now refused.
	if _, err := d.Exec(`INSERT INTO subscriptions (user_id, plan, provider_reference, status) VALUES (1, 'pro', 'r5', 'active')`); err == nil {
		t.Fatal("second current row was accepted")
	}
	// The stale cancelled row of user 2 is picked up by the expiry sweep query.
	exp, err := NewSubscriptionRepository(d).ListExpiredSubscriptions(context.Background(), "2026-01-15 00:00:00", 10)
	if err != nil || len(exp) != 1 || exp[0].ProviderReference != "r4" {
		t.Fatalf("expired = %+v, %v", exp, err)
	}
}
