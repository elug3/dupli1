// backfill-sku-id is a one-off migration tool that resolves the historical
// sku string on order_items/checkout_session_items rows to the canonical
// sku_id (ULID) minted by the product service, by calling product's public
// GET /api/v1/variants/{sku} lookup for each distinct legacy sku.
//
// It is idempotent (safe to re-run — only touches rows where sku_id IS NULL)
// and reports orphans (a sku with no matching product variant, e.g. a
// hard-deleted variant) without failing the run. Once every row across both
// tables has a sku_id, order's own migrate() promotes the primary key to
// (order_id|session_id, sku_id) automatically on next startup.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
)

func main() {
	fs := flag.NewFlagSet("backfill-sku-id", flag.ExitOnError)
	orderDB := fs.String("order-db", os.Getenv("DUPLI1_ORDER_DB"), "order database connection string (also DUPLI1_ORDER_DB env)")
	productURL := fs.String("product-url", os.Getenv("DUPLI1_PRODUCT_URL"), "product service base URL (also DUPLI1_PRODUCT_URL env)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
	if *orderDB == "" || *productURL == "" {
		fmt.Fprintln(os.Stderr, "both -order-db and -product-url are required")
		fs.Usage()
		os.Exit(1)
	}

	ctx := context.Background()

	conn, err := pgx.Connect(ctx, *orderDB)
	if err != nil {
		log.Fatalf("connect order db: %v", err)
	}
	defer conn.Close(ctx)

	httpClient := &http.Client{Timeout: 5 * time.Second}
	baseURL := strings.TrimRight(*productURL, "/")

	skus, err := distinctUnresolvedSKUs(ctx, conn)
	if err != nil {
		log.Fatalf("list unresolved skus: %v", err)
	}
	log.Printf("found %d distinct sku(s) missing sku_id", len(skus))

	var resolved, orphaned int
	for _, sku := range skus {
		skuID, err := lookupSkuID(ctx, httpClient, baseURL, sku)
		if err != nil {
			log.Printf("skip orphan sku %q: %v", sku, err)
			orphaned++
			continue
		}

		tag1, err := conn.Exec(ctx, `UPDATE order_items SET sku_id = $2 WHERE sku = $1 AND sku_id IS NULL`, sku, skuID)
		if err != nil {
			log.Fatalf("update order_items for sku %q: %v", sku, err)
		}
		tag2, err := conn.Exec(ctx, `UPDATE checkout_session_items SET sku_id = $2 WHERE sku = $1 AND sku_id IS NULL`, sku, skuID)
		if err != nil {
			log.Fatalf("update checkout_session_items for sku %q: %v", sku, err)
		}
		log.Printf("resolved sku %q -> sku_id %q (%d order_items, %d checkout_session_items rows)",
			sku, skuID, tag1.RowsAffected(), tag2.RowsAffected())
		resolved++
	}

	log.Printf("done: resolved %d sku(s), %d orphaned (no matching product variant)", resolved, orphaned)
	if orphaned > 0 {
		log.Printf("orphaned rows keep sku_id = NULL; order's primary-key promotion stays on the legacy key until every row resolves")
	} else {
		log.Printf("all rows resolved — order will promote its primary keys to (*, sku_id) on next service restart")
	}
}

func distinctUnresolvedSKUs(ctx context.Context, conn *pgx.Conn) ([]string, error) {
	rows, err := conn.Query(ctx, `
		SELECT DISTINCT sku FROM order_items WHERE sku_id IS NULL
		UNION
		SELECT DISTINCT sku FROM checkout_session_items WHERE sku_id IS NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var skus []string
	for rows.Next() {
		var sku string
		if err := rows.Scan(&sku); err != nil {
			return nil, err
		}
		skus = append(skus, sku)
	}
	return skus, rows.Err()
}

func lookupSkuID(ctx context.Context, httpClient *http.Client, baseURL, sku string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/products/variants/by-sku/"+sku, nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("no matching product variant")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("product request failed: %s", resp.Status)
	}

	var body struct {
		SkuID string `json:"skuId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.SkuID == "" {
		return "", fmt.Errorf("product response missing skuId")
	}
	return body.SkuID, nil
}
