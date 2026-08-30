package pg

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// CatalogStore implements ports.CatalogStore against PostgreSQL.
type CatalogStore struct {
	pool *pgxpool.Pool
}

func NewCatalogStore(pool *pgxpool.Pool) *CatalogStore {
	return &CatalogStore{pool: pool}
}

func (s *CatalogStore) ListBrands(ctx context.Context) ([]domain.Brand, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT code, name FROM brands ORDER BY code`)
	if err != nil {
		return nil, wrapDB("list brands", err)
	}
	defer rows.Close()
	var out []domain.Brand
	for rows.Next() {
		var b domain.Brand
		if err := rows.Scan(&b.Code, &b.Name); err != nil {
			return nil, wrapDB("list brands", err)
		}
		out = append(out, b)
	}
	return out, wrapDB("list brands", rows.Err())
}

func (s *CatalogStore) GetBrand(ctx context.Context, code string) (*domain.Brand, error) {
	var b domain.Brand
	err := s.pool.QueryRow(ctx,
		`SELECT code, name FROM brands WHERE code = $1`, domain.NormalizeCode(code),
	).Scan(&b.Code, &b.Name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrMasterNotFound
		}
		return nil, wrapDB("get brand", err)
	}
	return &b, nil
}

func (s *CatalogStore) CreateBrand(ctx context.Context, b domain.Brand) (*domain.Brand, error) {
	b.Code = domain.NormalizeCode(b.Code)
	b.Name = strings.TrimSpace(b.Name)
	if !domain.ValidBrandCode(b.Code) {
		return nil, ports.Invalid(fmt.Sprintf("invalid brand code %q", b.Code))
	}
	if b.Name == "" {
		return nil, ports.Invalid("name is required")
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO brands (code, name) VALUES ($1, $2)`, b.Code, b.Name)
	if isUniqueViolation(err) {
		return nil, domain.ErrMasterExists
	}
	if err != nil {
		return nil, wrapDB("create brand", err)
	}
	return &b, nil
}

func (s *CatalogStore) UpdateBrandName(ctx context.Context, code, name string) (*domain.Brand, error) {
	code = domain.NormalizeCode(code)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ports.Invalid("name is required")
	}
	cmd, err := s.pool.Exec(ctx,
		`UPDATE brands SET name = $2 WHERE code = $1`, code, name)
	if err != nil {
		return nil, wrapDB("update brand", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, domain.ErrMasterNotFound
	}
	return &domain.Brand{Code: code, Name: name}, nil
}

func (s *CatalogStore) DeleteBrand(ctx context.Context, code string) error {
	code = domain.NormalizeCode(code)
	cmd, err := s.pool.Exec(ctx, `DELETE FROM brands WHERE code = $1`, code)
	if isFKViolation(err) {
		return domain.ErrMasterInUse
	}
	if err != nil {
		return wrapDB("delete brand", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrMasterNotFound
	}
	return nil
}

func (s *CatalogStore) ListStyles(ctx context.Context, brandCode string) ([]domain.Style, error) {
	brandCode = domain.NormalizeCode(brandCode)
	rows, err := s.pool.Query(ctx,
		`SELECT brand_code, code, name FROM styles WHERE brand_code = $1 ORDER BY code`, brandCode)
	if err != nil {
		return nil, wrapDB("list styles", err)
	}
	defer rows.Close()
	var out []domain.Style
	for rows.Next() {
		var st domain.Style
		if err := rows.Scan(&st.BrandCode, &st.Code, &st.Name); err != nil {
			return nil, wrapDB("list styles", err)
		}
		out = append(out, st)
	}
	return out, wrapDB("list styles", rows.Err())
}

func (s *CatalogStore) GetStyle(ctx context.Context, brandCode, code string) (*domain.Style, error) {
	var st domain.Style
	err := s.pool.QueryRow(ctx,
		`SELECT brand_code, code, name FROM styles WHERE brand_code = $1 AND code = $2`,
		domain.NormalizeCode(brandCode), domain.NormalizeCode(code),
	).Scan(&st.BrandCode, &st.Code, &st.Name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrMasterNotFound
		}
		return nil, wrapDB("get style", err)
	}
	return &st, nil
}

func (s *CatalogStore) CreateStyle(ctx context.Context, st domain.Style) (*domain.Style, error) {
	st.BrandCode = domain.NormalizeCode(st.BrandCode)
	st.Code = domain.NormalizeCode(st.Code)
	st.Name = strings.TrimSpace(st.Name)
	if !domain.ValidBrandCode(st.BrandCode) {
		return nil, ports.Invalid(fmt.Sprintf("invalid brand code %q", st.BrandCode))
	}
	if !domain.ValidSegmentCode(st.Code) {
		return nil, ports.Invalid(fmt.Sprintf("invalid style code %q", st.Code))
	}
	if st.Name == "" {
		return nil, ports.Invalid("name is required")
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO styles (brand_code, code, name) VALUES ($1, $2, $3)`,
		st.BrandCode, st.Code, st.Name)
	if isUniqueViolation(err) {
		return nil, domain.ErrMasterExists
	}
	if err != nil {
		return nil, wrapDB("create style", err)
	}
	return &st, nil
}

func (s *CatalogStore) UpdateStyleName(ctx context.Context, brandCode, code, name string) (*domain.Style, error) {
	brandCode = domain.NormalizeCode(brandCode)
	code = domain.NormalizeCode(code)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ports.Invalid("name is required")
	}
	cmd, err := s.pool.Exec(ctx,
		`UPDATE styles SET name = $3 WHERE brand_code = $1 AND code = $2`,
		brandCode, code, name)
	if err != nil {
		return nil, wrapDB("update style", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, domain.ErrMasterNotFound
	}
	return &domain.Style{BrandCode: brandCode, Code: code, Name: name}, nil
}

func (s *CatalogStore) DeleteStyle(ctx context.Context, brandCode, code string) error {
	brandCode = domain.NormalizeCode(brandCode)
	code = domain.NormalizeCode(code)
	cmd, err := s.pool.Exec(ctx,
		`DELETE FROM styles WHERE brand_code = $1 AND code = $2`, brandCode, code)
	if isFKViolation(err) {
		return domain.ErrMasterInUse
	}
	if err != nil {
		return wrapDB("delete style", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrMasterNotFound
	}
	return nil
}

func (s *CatalogStore) ListColors(ctx context.Context) ([]domain.Color, error) {
	rows, err := s.pool.Query(ctx, `SELECT code, name FROM colors ORDER BY code`)
	if err != nil {
		return nil, wrapDB("list colors", err)
	}
	defer rows.Close()
	var out []domain.Color
	for rows.Next() {
		var c domain.Color
		if err := rows.Scan(&c.Code, &c.Name); err != nil {
			return nil, wrapDB("list colors", err)
		}
		out = append(out, c)
	}
	return out, wrapDB("list colors", rows.Err())
}

func (s *CatalogStore) GetColor(ctx context.Context, code string) (*domain.Color, error) {
	var c domain.Color
	err := s.pool.QueryRow(ctx,
		`SELECT code, name FROM colors WHERE code = $1`, domain.NormalizeCode(code),
	).Scan(&c.Code, &c.Name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrMasterNotFound
		}
		return nil, wrapDB("get color", err)
	}
	return &c, nil
}

func (s *CatalogStore) CreateColor(ctx context.Context, c domain.Color) (*domain.Color, error) {
	c.Code = domain.NormalizeCode(c.Code)
	c.Name = strings.TrimSpace(c.Name)
	if !domain.ValidSegmentCode(c.Code) {
		return nil, ports.Invalid(fmt.Sprintf("invalid color code %q", c.Code))
	}
	if c.Name == "" {
		return nil, ports.Invalid("name is required")
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO colors (code, name) VALUES ($1, $2)`, c.Code, c.Name)
	if isUniqueViolation(err) {
		return nil, domain.ErrMasterExists
	}
	if err != nil {
		return nil, wrapDB("create color", err)
	}
	return &c, nil
}

func (s *CatalogStore) UpdateColorName(ctx context.Context, code, name string) (*domain.Color, error) {
	code = domain.NormalizeCode(code)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ports.Invalid("name is required")
	}
	cmd, err := s.pool.Exec(ctx,
		`UPDATE colors SET name = $2 WHERE code = $1`, code, name)
	if err != nil {
		return nil, wrapDB("update color", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, domain.ErrMasterNotFound
	}
	return &domain.Color{Code: code, Name: name}, nil
}

func (s *CatalogStore) DeleteColor(ctx context.Context, code string) error {
	code = domain.NormalizeCode(code)
	cmd, err := s.pool.Exec(ctx, `DELETE FROM colors WHERE code = $1`, code)
	if isFKViolation(err) {
		return domain.ErrMasterInUse
	}
	if err != nil {
		return wrapDB("delete color", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrMasterNotFound
	}
	return nil
}

func (s *CatalogStore) ListSizes(ctx context.Context) ([]domain.Size, error) {
	rows, err := s.pool.Query(ctx, `SELECT code, name FROM sizes ORDER BY code`)
	if err != nil {
		return nil, wrapDB("list sizes", err)
	}
	defer rows.Close()
	var out []domain.Size
	for rows.Next() {
		var sz domain.Size
		if err := rows.Scan(&sz.Code, &sz.Name); err != nil {
			return nil, wrapDB("list sizes", err)
		}
		out = append(out, sz)
	}
	return out, wrapDB("list sizes", rows.Err())
}

func (s *CatalogStore) GetSize(ctx context.Context, code string) (*domain.Size, error) {
	var sz domain.Size
	err := s.pool.QueryRow(ctx,
		`SELECT code, name FROM sizes WHERE code = $1`, domain.NormalizeCode(code),
	).Scan(&sz.Code, &sz.Name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrMasterNotFound
		}
		return nil, wrapDB("get size", err)
	}
	return &sz, nil
}

func (s *CatalogStore) CreateSize(ctx context.Context, sz domain.Size) (*domain.Size, error) {
	sz.Code = domain.NormalizeCode(sz.Code)
	sz.Name = strings.TrimSpace(sz.Name)
	if !domain.ValidSegmentCode(sz.Code) {
		return nil, ports.Invalid(fmt.Sprintf("invalid size code %q", sz.Code))
	}
	if sz.Name == "" {
		return nil, ports.Invalid("name is required")
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sizes (code, name) VALUES ($1, $2)`, sz.Code, sz.Name)
	if isUniqueViolation(err) {
		return nil, domain.ErrMasterExists
	}
	if err != nil {
		return nil, wrapDB("create size", err)
	}
	return &sz, nil
}

func (s *CatalogStore) UpdateSizeName(ctx context.Context, code, name string) (*domain.Size, error) {
	code = domain.NormalizeCode(code)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ports.Invalid("name is required")
	}
	cmd, err := s.pool.Exec(ctx,
		`UPDATE sizes SET name = $2 WHERE code = $1`, code, name)
	if err != nil {
		return nil, wrapDB("update size", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, domain.ErrMasterNotFound
	}
	return &domain.Size{Code: code, Name: name}, nil
}

func (s *CatalogStore) DeleteSize(ctx context.Context, code string) error {
	code = domain.NormalizeCode(code)
	cmd, err := s.pool.Exec(ctx, `DELETE FROM sizes WHERE code = $1`, code)
	if isFKViolation(err) {
		return domain.ErrMasterInUse
	}
	if err != nil {
		return wrapDB("delete size", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrMasterNotFound
	}
	return nil
}

func (s *CatalogStore) ListEditions(ctx context.Context) ([]domain.Edition, error) {
	rows, err := s.pool.Query(ctx, `SELECT code, name FROM sku_editions ORDER BY code`)
	if err != nil {
		return nil, wrapDB("list editions", err)
	}
	defer rows.Close()
	var out []domain.Edition
	for rows.Next() {
		var e domain.Edition
		if err := rows.Scan(&e.Code, &e.Name); err != nil {
			return nil, wrapDB("list editions", err)
		}
		out = append(out, e)
	}
	return out, wrapDB("list editions", rows.Err())
}

func (s *CatalogStore) GetEdition(ctx context.Context, code string) (*domain.Edition, error) {
	var e domain.Edition
	err := s.pool.QueryRow(ctx,
		`SELECT code, name FROM sku_editions WHERE code = $1`, domain.NormalizeCode(code),
	).Scan(&e.Code, &e.Name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrMasterNotFound
		}
		return nil, wrapDB("get edition", err)
	}
	return &e, nil
}

func (s *CatalogStore) CreateEdition(ctx context.Context, e domain.Edition) (*domain.Edition, error) {
	e.Code = domain.NormalizeCode(e.Code)
	e.Name = strings.TrimSpace(e.Name)
	if !domain.ValidSegmentCode(e.Code) {
		return nil, ports.Invalid(fmt.Sprintf("invalid edition code %q", e.Code))
	}
	if e.Name == "" {
		return nil, ports.Invalid("name is required")
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sku_editions (code, name) VALUES ($1, $2)`, e.Code, e.Name)
	if isUniqueViolation(err) {
		return nil, domain.ErrMasterExists
	}
	if err != nil {
		return nil, wrapDB("create edition", err)
	}
	return &e, nil
}

func (s *CatalogStore) UpdateEditionName(ctx context.Context, code, name string) (*domain.Edition, error) {
	code = domain.NormalizeCode(code)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ports.Invalid("name is required")
	}
	cmd, err := s.pool.Exec(ctx,
		`UPDATE sku_editions SET name = $2 WHERE code = $1`, code, name)
	if err != nil {
		return nil, wrapDB("update edition", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, domain.ErrMasterNotFound
	}
	return &domain.Edition{Code: code, Name: name}, nil
}

func (s *CatalogStore) DeleteEdition(ctx context.Context, code string) error {
	code = domain.NormalizeCode(code)
	cmd, err := s.pool.Exec(ctx, `DELETE FROM sku_editions WHERE code = $1`, code)
	if isFKViolation(err) {
		return domain.ErrMasterInUse
	}
	if err != nil {
		return wrapDB("delete edition", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrMasterNotFound
	}
	return nil
}

func (s *CatalogStore) listCatalogTerms(ctx context.Context, table string) ([]domain.CatalogTerm, error) {
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT code, name FROM %s ORDER BY sort_order ASC, code ASC`, table))
	if err != nil {
		return nil, wrapDB("list "+table, err)
	}
	defer rows.Close()
	var out []domain.CatalogTerm
	for rows.Next() {
		var t domain.CatalogTerm
		if err := rows.Scan(&t.Code, &t.Name); err != nil {
			return nil, wrapDB("list "+table, err)
		}
		out = append(out, t)
	}
	return out, wrapDB("list "+table, rows.Err())
}

func (s *CatalogStore) ListSubCategories(ctx context.Context) ([]domain.CatalogTerm, error) {
	return s.listCatalogTerms(ctx, "bag_subcategories")
}

func (s *CatalogStore) ListBagStyles(ctx context.Context) ([]domain.CatalogTerm, error) {
	return s.listCatalogTerms(ctx, "bag_styles")
}

func (s *CatalogStore) ListTargets(ctx context.Context) ([]domain.CatalogTerm, error) {
	return s.listCatalogTerms(ctx, "bag_targets")
}
