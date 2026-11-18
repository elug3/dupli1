package pg

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

// BannerStore keeps storefront banners in storefront_banners. Implements
// ports.BannerStore.
type BannerStore struct {
	pool *pgxpool.Pool
}

func NewBannerStore(pool *pgxpool.Pool) (*BannerStore, error) {
	if _, err := pool.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS storefront_banners (
			id               TEXT PRIMARY KEY,
			placement        TEXT NOT NULL,
			image_url        TEXT NOT NULL,
			mobile_image_url TEXT NOT NULL DEFAULT '',
			title            JSONB NOT NULL DEFAULT '{}',
			link_url         TEXT NOT NULL DEFAULT '',
			link_label       JSONB NOT NULL DEFAULT '{}',
			active           BOOLEAN NOT NULL DEFAULT FALSE,
			sort_order       INTEGER NOT NULL DEFAULT 0,
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_by       TEXT NOT NULL DEFAULT ''
		)
	`); err != nil {
		return nil, fmt.Errorf("migrate storefront_banners: %w", err)
	}
	return &BannerStore{pool: pool}, nil
}

const bannerColumns = `id, placement, image_url, mobile_image_url, title, link_url, link_label,
	active, sort_order, created_at, updated_at, updated_by`

func scanBanner(row pgx.Row) (domain.Banner, error) {
	var b domain.Banner
	var placement string
	var title, label []byte
	if err := row.Scan(&b.ID, &placement, &b.ImageURL, &b.MobileImageURL, &title, &b.LinkURL, &label,
		&b.Active, &b.SortOrder, &b.CreatedAt, &b.UpdatedAt, &b.UpdatedBy); err != nil {
		return b, err
	}
	b.Placement = domain.BannerPlacement(placement)
	if err := json.Unmarshal(title, &b.Title); err != nil {
		return b, fmt.Errorf("banner %s title: %w", b.ID, err)
	}
	if err := json.Unmarshal(label, &b.LinkLabel); err != nil {
		return b, fmt.Errorf("banner %s link_label: %w", b.ID, err)
	}
	return b, nil
}

func (s *BannerStore) ListBanners(ctx context.Context) ([]domain.Banner, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+bannerColumns+` FROM storefront_banners ORDER BY placement, sort_order, created_at`)
	if err != nil {
		return nil, wrapDB("list banners", err)
	}
	defer rows.Close()
	out := []domain.Banner{}
	for rows.Next() {
		b, err := scanBanner(rows)
		if err != nil {
			return nil, wrapDB("scan banner", err)
		}
		out = append(out, b)
	}
	return out, wrapDB("list banners", rows.Err())
}

func (s *BannerStore) GetBanner(ctx context.Context, id string) (*domain.Banner, error) {
	b, err := scanBanner(s.pool.QueryRow(ctx, `SELECT `+bannerColumns+` FROM storefront_banners WHERE id = $1`, id))
	if err != nil {
		return nil, wrapDB("get banner", err)
	}
	return &b, nil
}

func bannerTexts(b domain.Banner) ([]byte, []byte, error) {
	title, err := json.Marshal(b.Title)
	if err != nil {
		return nil, nil, err
	}
	label, err := json.Marshal(b.LinkLabel)
	return title, label, err
}

func (s *BannerStore) CreateBanner(ctx context.Context, b domain.Banner) error {
	title, label, err := bannerTexts(b)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO storefront_banners (`+bannerColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		b.ID, string(b.Placement), b.ImageURL, b.MobileImageURL, title, b.LinkURL, label,
		b.Active, b.SortOrder, b.CreatedAt, b.UpdatedAt, b.UpdatedBy)
	return wrapDB("create banner", err)
}

func (s *BannerStore) UpdateBanner(ctx context.Context, b domain.Banner) error {
	title, label, err := bannerTexts(b)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE storefront_banners
		   SET placement = $2, image_url = $3, mobile_image_url = $4, title = $5, link_url = $6,
		       link_label = $7, active = $8, sort_order = $9, updated_at = $10, updated_by = $11
		 WHERE id = $1`,
		b.ID, string(b.Placement), b.ImageURL, b.MobileImageURL, title, b.LinkURL, label,
		b.Active, b.SortOrder, b.UpdatedAt, b.UpdatedBy)
	if err != nil {
		return wrapDB("update banner", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.NotFound("banner " + b.ID)
	}
	return nil
}

func (s *BannerStore) DeleteBanner(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM storefront_banners WHERE id = $1`, id)
	if err != nil {
		return wrapDB("delete banner", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.NotFound("banner " + id)
	}
	return nil
}
