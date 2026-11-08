package pg

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

// ReviewStore implements ports.ReviewStore on product's database
// (docs/product-reviews-plan.md).
//
// Every write locks the parent's products row and then recomputes its review
// counters from the rows in the same transaction. The lock serialises writers
// on one product, so the recount always sees the other writer's committed
// rows, and the counters are exact rather than incremented and drifting.
type ReviewStore struct {
	pool *pgxpool.Pool
}

func NewReviewStore(pool *pgxpool.Pool) (*ReviewStore, error) {
	s := &ReviewStore{pool: pool}
	if err := s.migrate(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *ReviewStore) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS product_reviews (
			id             TEXT PRIMARY KEY,
			product_id     TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
			sku_id         TEXT NOT NULL,
			order_id       TEXT NOT NULL,
			customer_id    TEXT NOT NULL,
			variant_label  TEXT NOT NULL DEFAULT '',
			author_display TEXT NOT NULL,
			rating         SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
			body           TEXT NOT NULL,
			fit            JSONB,
			hidden         BOOLEAN NOT NULL DEFAULT FALSE,
			hidden_reason  TEXT NOT NULL DEFAULT '',
			hidden_by      TEXT NOT NULL DEFAULT '',
			hidden_at      TIMESTAMPTZ,
			created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			edited_at      TIMESTAMPTZ,
			UNIQUE (order_id, sku_id)
		)`,
		`CREATE INDEX IF NOT EXISTS product_reviews_product_idx ON product_reviews (product_id, hidden, id DESC)`,
		`CREATE INDEX IF NOT EXISTS product_reviews_product_rating_idx ON product_reviews (product_id, hidden, rating, id DESC)`,
		`CREATE INDEX IF NOT EXISTS product_reviews_customer_idx ON product_reviews (customer_id, id DESC)`,
		`CREATE INDEX IF NOT EXISTS product_reviews_moderation_idx ON product_reviews (hidden, id DESC)`,
		`CREATE TABLE IF NOT EXISTS product_review_photos (
			id          TEXT PRIMARY KEY,
			review_id   TEXT REFERENCES product_reviews(id) ON DELETE CASCADE,
			customer_id TEXT NOT NULL,
			object_key  TEXT NOT NULL,
			thumb_key   TEXT NOT NULL,
			position    SMALLINT NOT NULL DEFAULT 0,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS product_review_photos_review_idx ON product_review_photos (review_id, position)`,
		`CREATE INDEX IF NOT EXISTS product_review_photos_customer_idx ON product_review_photos (customer_id)`,
		`CREATE INDEX IF NOT EXISTS product_review_photos_orphan_idx ON product_review_photos (created_at) WHERE review_id IS NULL`,
	}
	for _, col := range reviewCounterColumns {
		stmts = append(stmts, `ALTER TABLE products ADD COLUMN IF NOT EXISTS `+col+` BIGINT NOT NULL DEFAULT 0`)
	}
	for _, stmt := range stmts {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("migrate product reviews: %w", err)
		}
	}
	return nil
}

// reviewCounterColumns are the denormalized review counters on products.
var reviewCounterColumns = []string{
	"review_count", "review_rating_sum",
	"review_r1", "review_r2", "review_r3", "review_r4", "review_r5",
	"review_photo_count",
	"review_fit_small", "review_fit_true", "review_fit_large",
}

const reviewCols = `r.id, r.product_id, r.sku_id, r.order_id, r.customer_id, r.variant_label, r.author_display,
	r.rating, r.body, r.fit, r.hidden, r.hidden_reason, r.hidden_by, r.hidden_at, r.created_at, r.edited_at`

func scanReview(row pgx.Row) (domain.Review, error) {
	var r domain.Review
	var fitRaw []byte
	var rating int16
	var hiddenAt, editedAt *time.Time
	if err := row.Scan(&r.ID, &r.ProductID, &r.SkuID, &r.OrderID, &r.CustomerID, &r.VariantLabel, &r.Author,
		&rating, &r.Body, &fitRaw, &r.Hidden, &r.HiddenReason, &r.HiddenBy, &hiddenAt, &r.CreatedAt, &editedAt); err != nil {
		return domain.Review{}, err
	}
	r.Rating = int(rating)
	if len(fitRaw) > 0 {
		var fit domain.ReviewFit
		if err := json.Unmarshal(fitRaw, &fit); err == nil && fit.Result != "" {
			r.Fit = &fit
		}
	}
	r.HiddenAt = utcPtr(hiddenAt)
	r.EditedAt = utcPtr(editedAt)
	r.CreatedAt = r.CreatedAt.UTC()
	r.VerifiedPurchase = true
	return r, nil
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func fitJSON(fit *domain.ReviewFit) []byte {
	if fit == nil {
		return nil
	}
	raw, _ := json.Marshal(fit)
	return raw
}

// lockProducts takes the products rows of ids, in a fixed order so two
// writers on overlapping products cannot deadlock.
func lockProducts(ctx context.Context, tx pgx.Tx, ids ...string) error {
	uniq := map[string]bool{}
	for _, id := range ids {
		uniq[id] = true
	}
	sorted := make([]string, 0, len(uniq))
	for id := range uniq {
		sorted = append(sorted, id)
	}
	sort.Strings(sorted)
	for _, id := range sorted {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM products WHERE id = $1 FOR UPDATE`, id); err != nil {
			return wrapDB("lock product", err)
		}
	}
	return nil
}

// recountProduct recomputes a product's review counters from its visible
// reviews. Must run after lockProducts in the same transaction.
func recountProduct(ctx context.Context, tx pgx.Tx, productID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE products p SET
			review_count       = a.cnt,
			review_rating_sum  = a.total,
			review_r1 = a.r1, review_r2 = a.r2, review_r3 = a.r3, review_r4 = a.r4, review_r5 = a.r5,
			review_photo_count = a.photos,
			review_fit_small = a.fs, review_fit_true = a.ft, review_fit_large = a.fl
		FROM (
			SELECT COUNT(*)                                       AS cnt,
			       COALESCE(SUM(rating), 0)                       AS total,
			       COUNT(*) FILTER (WHERE rating = 1)             AS r1,
			       COUNT(*) FILTER (WHERE rating = 2)             AS r2,
			       COUNT(*) FILTER (WHERE rating = 3)             AS r3,
			       COUNT(*) FILTER (WHERE rating = 4)             AS r4,
			       COUNT(*) FILTER (WHERE rating = 5)             AS r5,
			       COUNT(*) FILTER (WHERE fit->>'result' = 'small')        AS fs,
			       COUNT(*) FILTER (WHERE fit->>'result' = 'true_to_size') AS ft,
			       COUNT(*) FILTER (WHERE fit->>'result' = 'large')        AS fl,
			       (SELECT COUNT(*) FROM product_review_photos ph
			          JOIN product_reviews v ON v.id = ph.review_id
			         WHERE v.product_id = $1 AND NOT v.hidden)  AS photos
			  FROM product_reviews
			 WHERE product_id = $1 AND NOT hidden
		) a
		WHERE p.id = $1`, productID)
	return wrapDB("recount reviews", err)
}

// attachPhotos gives photoIDs, in order, to reviewID. Each must be the
// customer's and either unattached or already this review's.
func attachPhotos(ctx context.Context, tx pgx.Tx, reviewID, customerID string, photoIDs []string) error {
	for i, id := range photoIDs {
		tag, err := tx.Exec(ctx, `
			UPDATE product_review_photos SET review_id = $1, position = $2
			 WHERE id = $3 AND customer_id = $4 AND (review_id IS NULL OR review_id = $1)`,
			reviewID, i, id, customerID)
		if err != nil {
			return wrapDB("attach review photo", err)
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("photo %s: %w", id, ports.ErrReviewPhotoUnusable)
		}
	}
	return nil
}

func (s *ReviewStore) CreateReview(ctx context.Context, r *domain.Review, photoIDs []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return wrapDB("begin", err)
	}
	defer tx.Rollback(ctx)

	if err := lockProducts(ctx, tx, r.ProductID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO product_reviews (id, product_id, sku_id, order_id, customer_id, variant_label, author_display, rating, body, fit, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		r.ID, r.ProductID, r.SkuID, r.OrderID, r.CustomerID, r.VariantLabel, r.Author, r.Rating, r.Body, fitJSON(r.Fit), r.CreatedAt)
	if isUniqueViolation(err) {
		var existing string
		_ = s.pool.QueryRow(ctx, `SELECT id FROM product_reviews WHERE order_id = $1 AND sku_id = $2`, r.OrderID, r.SkuID).Scan(&existing)
		return &ports.ReviewExistsError{ReviewID: existing}
	}
	if err != nil {
		return wrapDB("insert review", err)
	}
	if err := attachPhotos(ctx, tx, r.ID, r.CustomerID, photoIDs); err != nil {
		return err
	}
	if err := recountProduct(ctx, tx, r.ProductID); err != nil {
		return err
	}
	return wrapDB("commit review", tx.Commit(ctx))
}

func (s *ReviewStore) GetReview(ctx context.Context, id string) (*domain.Review, error) {
	r, err := scanReview(s.pool.QueryRow(ctx, `SELECT `+reviewCols+` FROM product_reviews r WHERE r.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ports.ErrReviewNotFound
	}
	if err != nil {
		return nil, wrapDB("get review", err)
	}
	reviews := []domain.Review{r}
	if err := s.loadPhotos(ctx, reviews); err != nil {
		return nil, err
	}
	return &reviews[0], nil
}

func (s *ReviewStore) UpdateReview(ctx context.Context, r *domain.Review, photoIDs []string, replacePhotos bool) ([]domain.ReviewPhoto, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, wrapDB("begin", err)
	}
	defer tx.Rollback(ctx)

	if err := lockProducts(ctx, tx, r.ProductID); err != nil {
		return nil, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE product_reviews SET rating = $2, body = $3, fit = $4, edited_at = $5
		 WHERE id = $1 AND NOT hidden`,
		r.ID, r.Rating, r.Body, fitJSON(r.Fit), r.EditedAt)
	if err != nil {
		return nil, wrapDB("update review", err)
	}
	if tag.RowsAffected() != 1 {
		return nil, ports.ErrReviewNotFound
	}

	var detached []domain.ReviewPhoto
	if replacePhotos {
		rows, err := tx.Query(ctx, `
			DELETE FROM product_review_photos
			 WHERE review_id = $1 AND NOT (id = ANY($2))
			RETURNING id, object_key, thumb_key`, r.ID, nonNilIDs(photoIDs))
		if err != nil {
			return nil, wrapDB("detach review photos", err)
		}
		for rows.Next() {
			var p domain.ReviewPhoto
			if err := rows.Scan(&p.ID, &p.ObjectKey, &p.ThumbKey); err != nil {
				rows.Close()
				return nil, wrapDB("scan detached photo", err)
			}
			detached = append(detached, p)
		}
		rows.Close()
		if err := attachPhotos(ctx, tx, r.ID, r.CustomerID, photoIDs); err != nil {
			return nil, err
		}
	}
	if err := recountProduct(ctx, tx, r.ProductID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, wrapDB("commit review", err)
	}
	return detached, nil
}

func nonNilIDs(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}

func (s *ReviewStore) DeleteReview(ctx context.Context, id string) ([]domain.ReviewPhoto, error) {
	var productID string
	if err := s.pool.QueryRow(ctx, `SELECT product_id FROM product_reviews WHERE id = $1`, id).Scan(&productID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrReviewNotFound
		}
		return nil, wrapDB("find review", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, wrapDB("begin", err)
	}
	defer tx.Rollback(ctx)
	if err := lockProducts(ctx, tx, productID); err != nil {
		return nil, err
	}
	photos, err := deletePhotosWhere(ctx, tx, `review_id = $1`, id)
	if err != nil {
		return nil, err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM product_reviews WHERE id = $1`, id)
	if err != nil {
		return nil, wrapDB("delete review", err)
	}
	if tag.RowsAffected() != 1 {
		return nil, ports.ErrReviewNotFound
	}
	if err := recountProduct(ctx, tx, productID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, wrapDB("commit delete review", err)
	}
	return photos, nil
}

func deletePhotosWhere(ctx context.Context, tx pgx.Tx, where string, args ...any) ([]domain.ReviewPhoto, error) {
	rows, err := tx.Query(ctx, `DELETE FROM product_review_photos WHERE `+where+` RETURNING id, object_key, thumb_key`, args...)
	if err != nil {
		return nil, wrapDB("delete review photos", err)
	}
	defer rows.Close()
	var out []domain.ReviewPhoto
	for rows.Next() {
		var p domain.ReviewPhoto
		if err := rows.Scan(&p.ID, &p.ObjectKey, &p.ThumbKey); err != nil {
			return nil, wrapDB("scan review photo", err)
		}
		out = append(out, p)
	}
	return out, wrapDB("delete review photos", rows.Err())
}

func (s *ReviewStore) SetReviewHidden(ctx context.Context, id string, hidden bool, reason, by string, at time.Time) (*domain.Review, error) {
	var productID string
	if err := s.pool.QueryRow(ctx, `SELECT product_id FROM product_reviews WHERE id = $1`, id).Scan(&productID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrReviewNotFound
		}
		return nil, wrapDB("find review", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, wrapDB("begin", err)
	}
	defer tx.Rollback(ctx)
	if err := lockProducts(ctx, tx, productID); err != nil {
		return nil, err
	}
	var hiddenAt *time.Time
	if hidden {
		hiddenAt = &at
	}
	if hidden {
		_, err = tx.Exec(ctx, `UPDATE product_reviews SET hidden = TRUE, hidden_reason = $2, hidden_by = $3, hidden_at = $4 WHERE id = $1`,
			id, reason, by, hiddenAt)
	} else {
		_, err = tx.Exec(ctx, `UPDATE product_reviews SET hidden = FALSE, hidden_reason = '', hidden_by = '', hidden_at = NULL WHERE id = $1`, id)
	}
	if err != nil {
		return nil, wrapDB("hide review", err)
	}
	if err := recountProduct(ctx, tx, productID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, wrapDB("commit hide review", err)
	}
	return s.GetReview(ctx, id)
}

// ── Reads ─────────────────────────────────────────────────────────────────

func (s *ReviewStore) ListProductReviews(ctx context.Context, productID string, q domain.ReviewQuery) (ports.ReviewPage, error) {
	where := []string{"r.product_id = $1", "NOT r.hidden"}
	args := []any{productID}
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	if q.Rating > 0 {
		where = append(where, "r.rating = "+arg(q.Rating))
	}
	if q.PhotoOnly {
		where = append(where, "EXISTS (SELECT 1 FROM product_review_photos ph WHERE ph.review_id = r.id)")
	}
	order := "r.id DESC"
	if q.Cursor != "" {
		rating, id, ok := decodeReviewCursor(q.Cursor)
		if !ok {
			return ports.ReviewPage{}, ports.Invalid("bad cursor")
		}
		switch q.Sort {
		case domain.ReviewSortRatingDesc:
			where = append(where, "(r.rating < "+arg(rating)+" OR (r.rating = $"+strconv.Itoa(len(args))+" AND r.id < "+arg(id)+"))")
		case domain.ReviewSortRatingAsc:
			where = append(where, "(r.rating > "+arg(rating)+" OR (r.rating = $"+strconv.Itoa(len(args))+" AND r.id < "+arg(id)+"))")
		default:
			where = append(where, "r.id < "+arg(id))
		}
	}
	switch q.Sort {
	case domain.ReviewSortRatingDesc:
		order = "r.rating DESC, r.id DESC"
	case domain.ReviewSortRatingAsc:
		order = "r.rating ASC, r.id DESC"
	}
	query := `SELECT ` + reviewCols + ` FROM product_reviews r WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY ` + order + ` LIMIT ` + arg(q.Limit+1)
	return s.page(ctx, query, args, q.Limit, true)
}

func (s *ReviewStore) ListReviewsForModeration(ctx context.Context, q domain.ModerationQuery) (ports.ReviewPage, error) {
	args := []any{q.Hidden}
	where := []string{"r.hidden = $1"}
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	if q.ProductID != "" {
		where = append(where, "r.product_id = "+arg(q.ProductID))
	}
	if q.Rating > 0 {
		where = append(where, "r.rating = "+arg(q.Rating))
	}
	if q.Cursor != "" {
		_, id, ok := decodeReviewCursor(q.Cursor)
		if !ok {
			return ports.ReviewPage{}, ports.Invalid("bad cursor")
		}
		where = append(where, "r.id < "+arg(id))
	}
	query := `SELECT ` + reviewCols + ` FROM product_reviews r WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY r.id DESC LIMIT ` + arg(q.Limit+1)
	return s.page(ctx, query, args, q.Limit, false)
}

// page runs a review query that asked for limit+1 rows.
func (s *ReviewStore) page(ctx context.Context, query string, args []any, limit int, ratingCursor bool) (ports.ReviewPage, error) {
	reviews, err := s.queryReviews(ctx, query, args...)
	if err != nil {
		return ports.ReviewPage{}, err
	}
	var next string
	if len(reviews) > limit {
		reviews = reviews[:limit]
		last := reviews[len(reviews)-1]
		next = encodeReviewCursor(last.Rating, last.ID)
	}
	if err := s.loadPhotos(ctx, reviews); err != nil {
		return ports.ReviewPage{}, err
	}
	return ports.ReviewPage{Reviews: reviews, NextCursor: next}, nil
}

func (s *ReviewStore) queryReviews(ctx context.Context, query string, args ...any) ([]domain.Review, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, wrapDB("list reviews", err)
	}
	defer rows.Close()
	var out []domain.Review
	for rows.Next() {
		r, err := scanReview(rows)
		if err != nil {
			return nil, wrapDB("scan review", err)
		}
		out = append(out, r)
	}
	return out, wrapDB("list reviews", rows.Err())
}

// loadPhotos fills each review's photos, in position order.
func (s *ReviewStore) loadPhotos(ctx context.Context, reviews []domain.Review) error {
	if len(reviews) == 0 {
		return nil
	}
	ids := make([]string, len(reviews))
	index := make(map[string]int, len(reviews))
	for i, r := range reviews {
		ids[i] = r.ID
		index[r.ID] = i
		reviews[i].Photos = []domain.ReviewPhoto{}
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, review_id, customer_id, object_key, thumb_key, created_at
		  FROM product_review_photos WHERE review_id = ANY($1)
		 ORDER BY review_id, position`, ids)
	if err != nil {
		return wrapDB("load review photos", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p domain.ReviewPhoto
		if err := rows.Scan(&p.ID, &p.ReviewID, &p.CustomerID, &p.ObjectKey, &p.ThumbKey, &p.CreatedAt); err != nil {
			return wrapDB("scan review photo", err)
		}
		i := index[p.ReviewID]
		reviews[i].Photos = append(reviews[i].Photos, p)
	}
	return wrapDB("load review photos", rows.Err())
}

func (s *ReviewStore) ListProductReviewPhotos(ctx context.Context, productID string, limit int, cursor string) (ports.ReviewPhotoPage, error) {
	args := []any{productID}
	where := ""
	if cursor != "" {
		pos, reviewID, ok := decodeReviewCursor(cursor)
		if !ok {
			return ports.ReviewPhotoPage{}, ports.Invalid("bad cursor")
		}
		args = append(args, reviewID, pos)
		where = ` AND (ph.review_id < $2 OR (ph.review_id = $2 AND ph.position > $3))`
	}
	args = append(args, limit+1)
	rows, err := s.pool.Query(ctx, `
		SELECT ph.id, ph.review_id, ph.customer_id, ph.object_key, ph.thumb_key, ph.position, ph.created_at
		  FROM product_review_photos ph
		  JOIN product_reviews r ON r.id = ph.review_id
		 WHERE r.product_id = $1 AND NOT r.hidden`+where+`
		 ORDER BY ph.review_id DESC, ph.position ASC
		 LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return ports.ReviewPhotoPage{}, wrapDB("list review photos", err)
	}
	defer rows.Close()
	var photos []domain.ReviewPhoto
	var positions []int
	for rows.Next() {
		var p domain.ReviewPhoto
		var pos int16
		if err := rows.Scan(&p.ID, &p.ReviewID, &p.CustomerID, &p.ObjectKey, &p.ThumbKey, &pos, &p.CreatedAt); err != nil {
			return ports.ReviewPhotoPage{}, wrapDB("scan review photo", err)
		}
		photos = append(photos, p)
		positions = append(positions, int(pos))
	}
	if err := rows.Err(); err != nil {
		return ports.ReviewPhotoPage{}, wrapDB("list review photos", err)
	}
	var next string
	if len(photos) > limit {
		photos = photos[:limit]
		last := photos[limit-1]
		next = encodeReviewCursor(positions[limit-1], last.ReviewID)
	}
	return ports.ReviewPhotoPage{Photos: photos, NextCursor: next}, nil
}

func (s *ReviewStore) ReviewCounters(ctx context.Context, productID string) (domain.ReviewCounters, error) {
	var c domain.ReviewCounters
	err := s.pool.QueryRow(ctx, `SELECT `+strings.Join(reviewCounterColumns, ", ")+` FROM products WHERE id = $1`, productID).Scan(
		&c.Count, &c.RatingSum,
		&c.RatingCounts[0], &c.RatingCounts[1], &c.RatingCounts[2], &c.RatingCounts[3], &c.RatingCounts[4],
		&c.PhotoCount,
		&c.FitCounts[0], &c.FitCounts[1], &c.FitCounts[2])
	if err != nil {
		return domain.ReviewCounters{}, wrapDB("review counters", err)
	}
	return c, nil
}

func (s *ReviewStore) ListCustomerReviews(ctx context.Context, customerID, orderID string) ([]domain.Review, error) {
	query := `SELECT ` + reviewCols + ` FROM product_reviews r WHERE r.customer_id = $1`
	args := []any{customerID}
	if orderID != "" {
		query += ` AND r.order_id = $2`
		args = append(args, orderID)
	}
	query += ` ORDER BY r.id DESC LIMIT 500`
	reviews, err := s.queryReviews(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if err := s.loadPhotos(ctx, reviews); err != nil {
		return nil, err
	}
	return reviews, nil
}

func (s *ReviewStore) CreateReviewPhoto(ctx context.Context, p *domain.ReviewPhoto) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO product_review_photos (id, customer_id, object_key, thumb_key, created_at)
		VALUES ($1, $2, $3, $4, $5)`, p.ID, p.CustomerID, p.ObjectKey, p.ThumbKey, p.CreatedAt)
	return wrapDB("create review photo", err)
}

func (s *ReviewStore) DeleteCustomerReviews(ctx context.Context, customerID string) ([]domain.ReviewPhoto, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, wrapDB("begin", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `SELECT DISTINCT product_id FROM product_reviews WHERE customer_id = $1`, customerID)
	if err != nil {
		return nil, wrapDB("find customer reviews", err)
	}
	var products []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, wrapDB("scan product id", err)
		}
		products = append(products, id)
	}
	rows.Close()
	if err := lockProducts(ctx, tx, products...); err != nil {
		return nil, err
	}
	// Photos first, attached or not, so the cascade from the reviews has
	// nothing left to remove silently: every object key comes back.
	photos, err := deletePhotosWhere(ctx, tx, `customer_id = $1`, customerID)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM product_reviews WHERE customer_id = $1`, customerID); err != nil {
		return nil, wrapDB("delete customer reviews", err)
	}
	for _, id := range products {
		if err := recountProduct(ctx, tx, id); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, wrapDB("commit customer review delete", err)
	}
	return photos, nil
}

func (s *ReviewStore) DeleteOrphanReviewPhotos(ctx context.Context, cutoff time.Time) ([]domain.ReviewPhoto, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, wrapDB("begin", err)
	}
	defer tx.Rollback(ctx)
	photos, err := deletePhotosWhere(ctx, tx, `review_id IS NULL AND created_at < $1`, cutoff)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, wrapDB("commit orphan sweep", err)
	}
	return photos, nil
}

// Cursors are opaque to clients: base64 of "<number>:<id>". The number is the
// rating for review lists and the photo position for photo lists.
func encodeReviewCursor(n int, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(n) + ":" + id))
}

func decodeReviewCursor(cursor string) (int, string, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, "", false
	}
	num, id, ok := strings.Cut(string(raw), ":")
	if !ok || id == "" {
		return 0, "", false
	}
	n, err := strconv.Atoi(num)
	if err != nil {
		return 0, "", false
	}
	return n, id, true
}
