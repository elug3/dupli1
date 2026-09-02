package pg

import (
	"os"
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/jackc/pgx/v4/pgxpool"
)

// requireDSN returns the test database DSN, skipping when Postgres is not available.
func requireDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		t.Skip("POSTGRES_URL not set; skipping postgres integration test")
	}
	return dsn
}

// freshSchema gives the test an empty schema so migrate() runs the way it does
// against a brand-new database. search_path is set on the pool config rather than
// with a SET statement, so it holds for every connection the pool opens.
func freshSchema(t *testing.T, dsn, schema string) *pgxpool.Pool {
	t.Helper()
	ctx := t.Context()

	admin, err := pgxpool.Connect(ctx, withPostgresSSLMode(dsn))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer admin.Close()
	for _, stmt := range []string{
		`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`,
		`CREATE SCHEMA ` + schema,
	} {
		if _, err := admin.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare schema (%s): %v", stmt, err)
		}
	}

	cfg, err := pgxpool.ParseConfig(withPostgresSSLMode(dsn))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect with search_path: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		cleanup, err := pgxpool.Connect(ctx, withPostgresSSLMode(dsn))
		if err != nil {
			return
		}
		defer cleanup.Close()
		_, _ = cleanup.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
	})
	return pool
}

// A fresh database has no payment_due_at column until the ALTER statements run, so
// anything depending on it must come after them or the service cannot start at all.
func TestMigrateOnEmptyDatabase(t *testing.T) {
	dsn := requireDSN(t)
	schema := "order_migrate_empty_test"
	pool := freshSchema(t, dsn, schema)
	repo := &Repository{pool: pool}

	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate on empty database returned error: %v", err)
	}

	ctx := t.Context()
	var columns int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = 'orders' AND column_name = 'payment_due_at'
	`, schema).Scan(&columns); err != nil {
		t.Fatalf("inspect orders columns: %v", err)
	}
	if columns != 1 {
		t.Fatalf("orders.payment_due_at column count = %d, want 1", columns)
	}

	var indexes int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_indexes
		WHERE schemaname = $1 AND tablename = 'orders'
		  AND indexname = 'idx_orders_pending_payment_due_at'
	`, schema).Scan(&indexes); err != nil {
		t.Fatalf("inspect orders indexes: %v", err)
	}
	if indexes != 1 {
		t.Fatalf("idx_orders_pending_payment_due_at count = %d, want 1", indexes)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn, "order_migrate_idempotent_test")
	repo := &Repository{pool: pool}

	if err := repo.migrate(); err != nil {
		t.Fatalf("first migrate returned error: %v", err)
	}
	if err := repo.migrate(); err != nil {
		t.Fatalf("second migrate returned error: %v", err)
	}
}

func TestSaveAndLoadOrderItemProductSnapshot(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn, "order_item_snapshot_test")
	repo := &Repository{pool: pool}
	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Millisecond)
	order, err := domain.NewOrder("ord-snap-1", "cust-1", "res-1", []domain.OrderItem{{
		SkuID:          "sku-bag-1",
		SKU:            "BAG-001",
		Quantity:       1,
		UnitPriceCents: 50000,
		ProductName:    "Prada Galleria",
		ImageURL:       "https://cdn.example/bag.jpg",
	}}, "", 0, now)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	if err := repo.Save(ctx, order); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := repo.Get(ctx, order.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(loaded.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(loaded.Items))
	}
	item := loaded.Items[0]
	if item.ProductName != "Prada Galleria" {
		t.Fatalf("ProductName = %q, want Prada Galleria", item.ProductName)
	}
	if item.ImageURL != "https://cdn.example/bag.jpg" {
		t.Fatalf("ImageURL = %q, want catalog image", item.ImageURL)
	}
}

func TestListAllReturnsOrdersAcrossCustomers(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn, "order_list_all_test")
	repo := &Repository{pool: pool}
	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Millisecond)
	for _, spec := range []struct {
		id, customer string
	}{
		{"ord-a", "cust-a"},
		{"ord-b", "cust-b"},
	} {
		order, err := domain.NewOrder(spec.id, spec.customer, "res-"+spec.id, []domain.OrderItem{{
			SkuID: "sku-" + spec.id, SKU: "BAG-001", Quantity: 1, UnitPriceCents: 1000,
		}}, "", 0, now)
		if err != nil {
			t.Fatalf("NewOrder(%s): %v", spec.id, err)
		}
		if err := repo.Save(ctx, order); err != nil {
			t.Fatalf("Save(%s): %v", spec.id, err)
		}
	}

	orders, err := repo.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(orders) != 2 {
		t.Fatalf("ListAll returned %d orders, want 2", len(orders))
	}
	customers := map[string]bool{}
	for _, o := range orders {
		customers[o.CustomerID] = true
	}
	if !customers["cust-a"] || !customers["cust-b"] {
		t.Fatalf("expected cust-a and cust-b, got %v", customers)
	}
}

func TestSaveAndLoadOrderShipmentTracking(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn, "order_shipment_tracking_test")
	repo := &Repository{pool: pool}
	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Millisecond)
	order, err := domain.NewOrder("ord-ship-1", "cust-1", "res-ship-1", []domain.OrderItem{{
		SkuID: "sku-ship-1", SKU: "BAG-001", Quantity: 1, UnitPriceCents: 50000,
	}}, "", 0, now)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	if err := order.MarkPaid("pay-ship-1", order.TotalCents, now); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	tracking := domain.ShipmentTracking{
		Carrier:        domain.CarrierOther,
		TrackingNumber: "INTL-TRACK-99",
		CarrierNote:    "DHL Express",
	}
	shipAt := now.Add(time.Minute)
	if err := order.Ship("manager-1", tracking, shipAt); err != nil {
		t.Fatalf("Ship: %v", err)
	}
	if err := repo.Save(ctx, order); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := repo.Get(ctx, order.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if loaded.Carrier != domain.CarrierOther {
		t.Fatalf("Carrier = %q, want other", loaded.Carrier)
	}
	if loaded.TrackingNumber != "INTL-TRACK-99" {
		t.Fatalf("TrackingNumber = %q", loaded.TrackingNumber)
	}
	if loaded.CarrierNote != "DHL Express" {
		t.Fatalf("CarrierNote = %q", loaded.CarrierNote)
	}
	if loaded.ShippedBy != "manager-1" || loaded.ShippedAt == nil {
		t.Fatalf("ship metadata = shipped_by %q shipped_at %v", loaded.ShippedBy, loaded.ShippedAt)
	}

	orders, err := repo.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("ListAll returned %d orders, want 1", len(orders))
	}
	if orders[0].TrackingNumber != "INTL-TRACK-99" {
		t.Fatalf("ListAll tracking = %q", orders[0].TrackingNumber)
	}
}
