// purge-products is a maintenance tool that removes every product from the
// product catalog (parent styles, their variants, and the attached inventory
// stock and reservations) so the catalog can be re-seeded from scratch.
//
// It exists because there is no bulk-delete HTTP endpoint, and per-product
// DELETE /api/v1/products/{id} is blocked whenever a variant still has a
// stock_items row (stock_items.sku_id references product_variants ON DELETE
// RESTRICT). This tool deletes the rows in FK-safe order inside a single
// transaction, so the operation is all-or-nothing.
//
// It defaults to a dry run that only reports current row counts; pass -confirm
// to actually delete. Coupons are left untouched.
//
// Usage:
//
//	DUPLI1_PRODUCT_DB=postgres://dupli1:dupli1_dev@localhost:5433/products?sslmode=disable \
//	    go run ./cmd/purge-products            # dry run: prints counts only
//	DUPLI1_PRODUCT_DB=... go run ./cmd/purge-products -confirm   # actually deletes
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v4"
)

// tables are listed in the order they must be deleted to satisfy foreign keys:
// reservations first (reservation_items cascades from it), then stock_items
// (which RESTRICTs deletion of product_variants), then products (product_variants
// cascades from it).
var deleteOrder = []string{"reservations", "stock_items", "products"}

// countTables are reported to the user before and after the purge.
var countTables = []string{"products", "product_variants", "stock_items", "reservations", "reservation_items"}

func main() {
	fs := flag.NewFlagSet("purge-products", flag.ExitOnError)
	productDB := fs.String("product-db", os.Getenv("DUPLI1_PRODUCT_DB"), "product database connection string (also DUPLI1_PRODUCT_DB env)")
	confirm := fs.Bool("confirm", false, "actually delete; without this flag the tool only reports counts (dry run)")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `Usage: purge-products [OPTIONS]

Removes every product, variant, stock row, and reservation from the product
catalog so it can be re-seeded. Coupons are not touched.

Options:
  -product-db string
      Product database connection string (also DUPLI1_PRODUCT_DB env)
  -confirm
      Actually delete. Without this flag the tool runs a dry run that only
      prints current row counts.
`)
	}
	if err := fs.Parse(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
	if *productDB == "" {
		fmt.Fprintln(os.Stderr, "-product-db (or DUPLI1_PRODUCT_DB) is required")
		fs.Usage()
		os.Exit(1)
	}

	ctx := context.Background()

	conn, err := pgx.Connect(ctx, *productDB)
	if err != nil {
		log.Fatalf("connect product db: %v", err)
	}
	defer conn.Close(ctx)

	before, err := counts(ctx, conn)
	if err != nil {
		log.Fatalf("count rows: %v", err)
	}
	log.Printf("current catalog: %s", format(before))

	if !*confirm {
		log.Printf("dry run: nothing deleted. Re-run with -confirm to remove all products.")
		return
	}

	deleted, err := purge(ctx, conn)
	if err != nil {
		log.Fatalf("purge: %v", err)
	}
	for _, t := range deleteOrder {
		log.Printf("deleted %d rows from %s (plus cascades)", deleted[t], t)
	}

	after, err := counts(ctx, conn)
	if err != nil {
		log.Fatalf("count rows after purge: %v", err)
	}
	log.Printf("catalog after purge: %s", format(after))
	log.Println("purge complete")
}

// purge deletes all catalog rows in one transaction and returns the number of
// rows removed by each top-level DELETE (cascaded child rows are not counted).
func purge(ctx context.Context, conn *pgx.Conn) (map[string]int64, error) {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	deleted := make(map[string]int64, len(deleteOrder))
	for _, t := range deleteOrder {
		tag, err := tx.Exec(ctx, "DELETE FROM "+t)
		if err != nil {
			return nil, fmt.Errorf("delete %s: %w", t, err)
		}
		deleted[t] = tag.RowsAffected()
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return deleted, nil
}

func counts(ctx context.Context, conn *pgx.Conn) (map[string]int64, error) {
	out := make(map[string]int64, len(countTables))
	for _, t := range countTables {
		var n int64
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM "+t).Scan(&n); err != nil {
			return nil, fmt.Errorf("count %s: %w", t, err)
		}
		out[t] = n
	}
	return out, nil
}

func format(c map[string]int64) string {
	return fmt.Sprintf("products=%d variants=%d stock_items=%d reservations=%d reservation_items=%d",
		c["products"], c["product_variants"], c["stock_items"], c["reservations"], c["reservation_items"])
}
