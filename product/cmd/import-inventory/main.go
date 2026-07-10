// import-inventory is a one-off migration tool that copies stock and
// reservation data from the standalone inventory service's database into
// product's merged inventory tables, remapping the legacy sku string to the
// canonical sku_id via product's own product_variants table.
//
// It is idempotent (safe to re-run) and skips inventory rows whose sku no
// longer exists in product_variants, logging them as orphans.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v4"
)

func main() {
	fs := flag.NewFlagSet("import-inventory", flag.ExitOnError)
	productDB := fs.String("product-db", os.Getenv("DUPLI1_PRODUCT_DB"), "product database connection string (also DUPLI1_PRODUCT_DB env)")
	inventoryDB := fs.String("inventory-db", os.Getenv("DUPLI1_INVENTORY_DB"), "legacy inventory database connection string (also DUPLI1_INVENTORY_DB env)")
	allReservations := fs.Bool("all-reservations", false, "import committed/released reservations too (default: active only)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
	if *productDB == "" || *inventoryDB == "" {
		fmt.Fprintln(os.Stderr, "both -product-db and -inventory-db are required")
		fs.Usage()
		os.Exit(1)
	}

	ctx := context.Background()

	product, err := pgx.Connect(ctx, *productDB)
	if err != nil {
		log.Fatalf("connect product db: %v", err)
	}
	defer product.Close(ctx)

	inventory, err := pgx.Connect(ctx, *inventoryDB)
	if err != nil {
		log.Fatalf("connect inventory db: %v", err)
	}
	defer inventory.Close(ctx)

	skuToID, err := loadSkuIDMap(ctx, product)
	if err != nil {
		log.Fatalf("load sku_id map: %v", err)
	}
	log.Printf("loaded %d sku -> sku_id mappings from product_variants", len(skuToID))

	if err := importStockItems(ctx, inventory, product, skuToID); err != nil {
		log.Fatalf("import stock_items: %v", err)
	}
	if err := importReservations(ctx, inventory, product, skuToID, *allReservations); err != nil {
		log.Fatalf("import reservations: %v", err)
	}
	log.Println("import complete")
}

func loadSkuIDMap(ctx context.Context, product *pgx.Conn) (map[string]string, error) {
	rows, err := product.Query(ctx, `SELECT sku, sku_id FROM product_variants`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[string]string)
	for rows.Next() {
		var sku, skuID string
		if err := rows.Scan(&sku, &skuID); err != nil {
			return nil, err
		}
		m[sku] = skuID
	}
	return m, rows.Err()
}

func importStockItems(ctx context.Context, inventory, product *pgx.Conn, skuToID map[string]string) error {
	rows, err := inventory.Query(ctx, `SELECT sku, quantity, reserved, updated_at FROM stock_items`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var imported, skipped int
	for rows.Next() {
		var sku string
		var quantity, reserved int
		var updatedAt any
		if err := rows.Scan(&sku, &quantity, &reserved, &updatedAt); err != nil {
			return err
		}
		skuID, ok := skuToID[sku]
		if !ok {
			log.Printf("skip orphan stock_items row: sku %q has no matching product_variants row", sku)
			skipped++
			continue
		}
		if _, err := product.Exec(ctx, `
			INSERT INTO stock_items (sku_id, quantity, reserved, updated_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (sku_id) DO NOTHING
		`, skuID, quantity, reserved, updatedAt); err != nil {
			return fmt.Errorf("insert stock_items for sku_id %s: %w", skuID, err)
		}
		imported++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	log.Printf("stock_items: imported %d, skipped %d orphans", imported, skipped)
	return nil
}

func importReservations(ctx context.Context, inventory, product *pgx.Conn, skuToID map[string]string, allReservations bool) error {
	query := `SELECT id, order_id, status, created_at, updated_at FROM reservations`
	if !allReservations {
		query += ` WHERE status = 'active'`
	}
	rows, err := inventory.Query(ctx, query)
	if err != nil {
		return err
	}
	type reservation struct {
		id, orderID, status  string
		createdAt, updatedAt any
	}
	var reservations []reservation
	for rows.Next() {
		var r reservation
		if err := rows.Scan(&r.id, &r.orderID, &r.status, &r.createdAt, &r.updatedAt); err != nil {
			rows.Close()
			return err
		}
		reservations = append(reservations, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	var imported, skipped int
	for _, r := range reservations {
		itemRows, err := inventory.Query(ctx, `SELECT sku, quantity FROM reservation_items WHERE reservation_id = $1`, r.id)
		if err != nil {
			return err
		}
		type item struct {
			sku      string
			skuID    string
			quantity int
		}
		var items []item
		orphaned := false
		for itemRows.Next() {
			var sku string
			var qty int
			if err := itemRows.Scan(&sku, &qty); err != nil {
				itemRows.Close()
				return err
			}
			skuID, ok := skuToID[sku]
			if !ok {
				log.Printf("skip reservation %s: item sku %q has no matching product_variants row", r.id, sku)
				orphaned = true
				continue
			}
			items = append(items, item{sku: sku, skuID: skuID, quantity: qty})
		}
		itemRows.Close()
		if err := itemRows.Err(); err != nil {
			return err
		}
		if orphaned || len(items) == 0 {
			skipped++
			continue
		}

		tx, err := product.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO reservations (id, order_id, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (id) DO NOTHING
		`, r.id, r.orderID, r.status, r.createdAt, r.updatedAt); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("insert reservation %s: %w", r.id, err)
		}
		for _, it := range items {
			if _, err := tx.Exec(ctx, `
				INSERT INTO reservation_items (reservation_id, sku_id, sku, quantity)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (reservation_id, sku_id) DO NOTHING
			`, r.id, it.skuID, it.sku, it.quantity); err != nil {
				tx.Rollback(ctx)
				return fmt.Errorf("insert reservation_items for %s: %w", r.id, err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		imported++
	}
	log.Printf("reservations: imported %d, skipped %d (orphaned or empty)", imported, skipped)
	return nil
}
