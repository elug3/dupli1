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

func (s *CatalogStore) ListBrands() ([]domain.Brand, error) {
	rows, err := s.pool.Query(context.Background(),
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

func (s *CatalogStore) GetBrand(code string) (*domain.Brand, error) {
	var b domain.Brand
	err := s.pool.QueryRow(context.Background(),
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

func (s *CatalogStore) CreateBrand(b domain.Brand) (*domain.Brand, error) {
	b.Code = domain.NormalizeCode(b.Code)
	b.Name = strings.TrimSpace(b.Name)
	if !domain.ValidBrandCode(b.Code) {
		return nil, ports.Invalid(fmt.Sprintf("invalid brand code %q", b.Code))
	}
	if b.Name == "" {
		return nil, ports.Invalid("name is required")
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO brands (code, name) VALUES ($1, $2)`, b.Code, b.Name)
	if isUniqueViolation(err) {
		return nil, domain.ErrMasterExists
	}
	if err != nil {
		return nil, wrapDB("create brand", err)
	}
	return &b, nil
}

func (s *CatalogStore) UpdateBrandName(code, name string) (*domain.Brand, error) {
	code = domain.NormalizeCode(code)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ports.Invalid("name is required")
	}
	cmd, err := s.pool.Exec(context.Background(),
		`UPDATE brands SET name = $2 WHERE code = $1`, code, name)
	if err != nil {
		return nil, wrapDB("update brand", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, domain.ErrMasterNotFound
	}
	return &domain.Brand{Code: code, Name: name}, nil
}

func (s *CatalogStore) DeleteBrand(code string) error {
	code = domain.NormalizeCode(code)
	cmd, err := s.pool.Exec(context.Background(), `DELETE FROM brands WHERE code = $1`, code)
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

func (s *CatalogStore) ListStyles(brandCode string) ([]domain.Style, error) {
	brandCode = domain.NormalizeCode(brandCode)
	rows, err := s.pool.Query(context.Background(),
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

func (s *CatalogStore) GetStyle(brandCode, code string) (*domain.Style, error) {
	var st domain.Style
	err := s.pool.QueryRow(context.Background(),
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

func (s *CatalogStore) CreateStyle(st domain.Style) (*domain.Style, error) {
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
	_, err := s.pool.Exec(context.Background(),
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

func (s *CatalogStore) UpdateStyleName(brandCode, code, name string) (*domain.Style, error) {
	brandCode = domain.NormalizeCode(brandCode)
	code = domain.NormalizeCode(code)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ports.Invalid("name is required")
	}
	cmd, err := s.pool.Exec(context.Background(),
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

func (s *CatalogStore) DeleteStyle(brandCode, code string) error {
	brandCode = domain.NormalizeCode(brandCode)
	code = domain.NormalizeCode(code)
	cmd, err := s.pool.Exec(context.Background(),
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

func (s *CatalogStore) ListColors() ([]domain.Color, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT code, name FROM colors ORDER BY code`)
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

func (s *CatalogStore) GetColor(code string) (*domain.Color, error) {
	var c domain.Color
	err := s.pool.QueryRow(context.Background(),
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

func (s *CatalogStore) CreateColor(c domain.Color) (*domain.Color, error) {
	c.Code = domain.NormalizeCode(c.Code)
	c.Name = strings.TrimSpace(c.Name)
	if !domain.ValidSegmentCode(c.Code) {
		return nil, ports.Invalid(fmt.Sprintf("invalid color code %q", c.Code))
	}
	if c.Name == "" {
		return nil, ports.Invalid("name is required")
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO colors (code, name) VALUES ($1, $2)`, c.Code, c.Name)
	if isUniqueViolation(err) {
		return nil, domain.ErrMasterExists
	}
	if err != nil {
		return nil, wrapDB("create color", err)
	}
	return &c, nil
}

func (s *CatalogStore) UpdateColorName(code, name string) (*domain.Color, error) {
	code = domain.NormalizeCode(code)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ports.Invalid("name is required")
	}
	cmd, err := s.pool.Exec(context.Background(),
		`UPDATE colors SET name = $2 WHERE code = $1`, code, name)
	if err != nil {
		return nil, wrapDB("update color", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, domain.ErrMasterNotFound
	}
	return &domain.Color{Code: code, Name: name}, nil
}

func (s *CatalogStore) DeleteColor(code string) error {
	code = domain.NormalizeCode(code)
	cmd, err := s.pool.Exec(context.Background(), `DELETE FROM colors WHERE code = $1`, code)
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

func (s *CatalogStore) ListSizes() ([]domain.Size, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT code, name FROM sizes ORDER BY code`)
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

func (s *CatalogStore) GetSize(code string) (*domain.Size, error) {
	var sz domain.Size
	err := s.pool.QueryRow(context.Background(),
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

func (s *CatalogStore) CreateSize(sz domain.Size) (*domain.Size, error) {
	sz.Code = domain.NormalizeCode(sz.Code)
	sz.Name = strings.TrimSpace(sz.Name)
	if !domain.ValidSegmentCode(sz.Code) {
		return nil, ports.Invalid(fmt.Sprintf("invalid size code %q", sz.Code))
	}
	if sz.Name == "" {
		return nil, ports.Invalid("name is required")
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO sizes (code, name) VALUES ($1, $2)`, sz.Code, sz.Name)
	if isUniqueViolation(err) {
		return nil, domain.ErrMasterExists
	}
	if err != nil {
		return nil, wrapDB("create size", err)
	}
	return &sz, nil
}

func (s *CatalogStore) UpdateSizeName(code, name string) (*domain.Size, error) {
	code = domain.NormalizeCode(code)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ports.Invalid("name is required")
	}
	cmd, err := s.pool.Exec(context.Background(),
		`UPDATE sizes SET name = $2 WHERE code = $1`, code, name)
	if err != nil {
		return nil, wrapDB("update size", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, domain.ErrMasterNotFound
	}
	return &domain.Size{Code: code, Name: name}, nil
}

func (s *CatalogStore) DeleteSize(code string) error {
	code = domain.NormalizeCode(code)
	cmd, err := s.pool.Exec(context.Background(), `DELETE FROM sizes WHERE code = $1`, code)
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

func (s *CatalogStore) ListEditions() ([]domain.Edition, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT code, name FROM sku_editions ORDER BY code`)
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

func (s *CatalogStore) GetEdition(code string) (*domain.Edition, error) {
	var e domain.Edition
	err := s.pool.QueryRow(context.Background(),
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

func (s *CatalogStore) CreateEdition(e domain.Edition) (*domain.Edition, error) {
	e.Code = domain.NormalizeCode(e.Code)
	e.Name = strings.TrimSpace(e.Name)
	if !domain.ValidSegmentCode(e.Code) {
		return nil, ports.Invalid(fmt.Sprintf("invalid edition code %q", e.Code))
	}
	if e.Name == "" {
		return nil, ports.Invalid("name is required")
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO sku_editions (code, name) VALUES ($1, $2)`, e.Code, e.Name)
	if isUniqueViolation(err) {
		return nil, domain.ErrMasterExists
	}
	if err != nil {
		return nil, wrapDB("create edition", err)
	}
	return &e, nil
}

func (s *CatalogStore) UpdateEditionName(code, name string) (*domain.Edition, error) {
	code = domain.NormalizeCode(code)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ports.Invalid("name is required")
	}
	cmd, err := s.pool.Exec(context.Background(),
		`UPDATE sku_editions SET name = $2 WHERE code = $1`, code, name)
	if err != nil {
		return nil, wrapDB("update edition", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, domain.ErrMasterNotFound
	}
	return &domain.Edition{Code: code, Name: name}, nil
}

func (s *CatalogStore) DeleteEdition(code string) error {
	code = domain.NormalizeCode(code)
	cmd, err := s.pool.Exec(context.Background(), `DELETE FROM sku_editions WHERE code = $1`, code)
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

func (s *CatalogStore) ListSubcategories() ([]domain.Subcategory, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT code, name FROM subcategories ORDER BY code`)
	if err != nil {
		return nil, wrapDB("list subcategories", err)
	}
	defer rows.Close()
	var out []domain.Subcategory
	for rows.Next() {
		var item domain.Subcategory
		if err := rows.Scan(&item.Code, &item.Name); err != nil {
			return nil, wrapDB("list subcategories", err)
		}
		out = append(out, item)
	}
	return out, wrapDB("list subcategories", rows.Err())
}

func (s *CatalogStore) GetSubcategory(code string) (*domain.Subcategory, error) {
	var item domain.Subcategory
	err := s.pool.QueryRow(context.Background(),
		`SELECT code, name FROM subcategories WHERE code = $1`, domain.NormalizeCode(code),
	).Scan(&item.Code, &item.Name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrMasterNotFound
		}
		return nil, wrapDB("get subcategory", err)
	}
	return &item, nil
}

func (s *CatalogStore) CreateSubcategory(item domain.Subcategory) (*domain.Subcategory, error) {
	item.Code = domain.NormalizeCode(item.Code)
	item.Name = strings.TrimSpace(item.Name)
	if !domain.ValidSegmentCode(item.Code) {
		return nil, ports.Invalid(fmt.Sprintf("invalid subcategory code %q", item.Code))
	}
	if item.Name == "" {
		return nil, ports.Invalid("name is required")
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO subcategories (code, name) VALUES ($1, $2)`, item.Code, item.Name)
	if isUniqueViolation(err) {
		return nil, domain.ErrMasterExists
	}
	if err != nil {
		return nil, wrapDB("create subcategory", err)
	}
	return &item, nil
}

func (s *CatalogStore) UpdateSubcategoryName(code, name string) (*domain.Subcategory, error) {
	code = domain.NormalizeCode(code)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ports.Invalid("name is required")
	}
	cmd, err := s.pool.Exec(context.Background(),
		`UPDATE subcategories SET name = $2 WHERE code = $1`, code, name)
	if err != nil {
		return nil, wrapDB("update subcategory", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, domain.ErrMasterNotFound
	}
	return &domain.Subcategory{Code: code, Name: name}, nil
}

func (s *CatalogStore) DeleteSubcategory(code string) error {
	code = domain.NormalizeCode(code)
	cmd, err := s.pool.Exec(context.Background(), `DELETE FROM subcategories WHERE code = $1`, code)
	if isFKViolation(err) {
		return domain.ErrMasterInUse
	}
	if err != nil {
		return wrapDB("delete subcategory", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrMasterNotFound
	}
	return nil
}

func (s *CatalogStore) ListOccasions() ([]domain.Occasion, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT code, name FROM occasions ORDER BY code`)
	if err != nil {
		return nil, wrapDB("list occasions", err)
	}
	defer rows.Close()
	var out []domain.Occasion
	for rows.Next() {
		var item domain.Occasion
		if err := rows.Scan(&item.Code, &item.Name); err != nil {
			return nil, wrapDB("list occasions", err)
		}
		out = append(out, item)
	}
	return out, wrapDB("list occasions", rows.Err())
}

func (s *CatalogStore) GetOccasion(code string) (*domain.Occasion, error) {
	var item domain.Occasion
	err := s.pool.QueryRow(context.Background(),
		`SELECT code, name FROM occasions WHERE code = $1`, domain.NormalizeCode(code),
	).Scan(&item.Code, &item.Name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrMasterNotFound
		}
		return nil, wrapDB("get occasion", err)
	}
	return &item, nil
}

func (s *CatalogStore) CreateOccasion(item domain.Occasion) (*domain.Occasion, error) {
	item.Code = domain.NormalizeCode(item.Code)
	item.Name = strings.TrimSpace(item.Name)
	if !domain.ValidSegmentCode(item.Code) {
		return nil, ports.Invalid(fmt.Sprintf("invalid occasion code %q", item.Code))
	}
	if item.Name == "" {
		return nil, ports.Invalid("name is required")
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO occasions (code, name) VALUES ($1, $2)`, item.Code, item.Name)
	if isUniqueViolation(err) {
		return nil, domain.ErrMasterExists
	}
	if err != nil {
		return nil, wrapDB("create occasion", err)
	}
	return &item, nil
}

func (s *CatalogStore) UpdateOccasionName(code, name string) (*domain.Occasion, error) {
	code = domain.NormalizeCode(code)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ports.Invalid("name is required")
	}
	cmd, err := s.pool.Exec(context.Background(),
		`UPDATE occasions SET name = $2 WHERE code = $1`, code, name)
	if err != nil {
		return nil, wrapDB("update occasion", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, domain.ErrMasterNotFound
	}
	return &domain.Occasion{Code: code, Name: name}, nil
}

func (s *CatalogStore) DeleteOccasion(code string) error {
	code = domain.NormalizeCode(code)
	cmd, err := s.pool.Exec(context.Background(), `DELETE FROM occasions WHERE code = $1`, code)
	if isFKViolation(err) {
		return domain.ErrMasterInUse
	}
	if err != nil {
		return wrapDB("delete occasion", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrMasterNotFound
	}
	return nil
}

func (s *CatalogStore) ListTargets() ([]domain.Target, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT code, name FROM targets ORDER BY code`)
	if err != nil {
		return nil, wrapDB("list targets", err)
	}
	defer rows.Close()
	var out []domain.Target
	for rows.Next() {
		var item domain.Target
		if err := rows.Scan(&item.Code, &item.Name); err != nil {
			return nil, wrapDB("list targets", err)
		}
		out = append(out, item)
	}
	return out, wrapDB("list targets", rows.Err())
}

func (s *CatalogStore) GetTarget(code string) (*domain.Target, error) {
	var item domain.Target
	err := s.pool.QueryRow(context.Background(),
		`SELECT code, name FROM targets WHERE code = $1`, domain.NormalizeCode(code),
	).Scan(&item.Code, &item.Name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrMasterNotFound
		}
		return nil, wrapDB("get target", err)
	}
	return &item, nil
}

func (s *CatalogStore) CreateTarget(item domain.Target) (*domain.Target, error) {
	item.Code = domain.NormalizeCode(item.Code)
	item.Name = strings.TrimSpace(item.Name)
	if !domain.ValidSegmentCode(item.Code) {
		return nil, ports.Invalid(fmt.Sprintf("invalid target code %q", item.Code))
	}
	if item.Name == "" {
		return nil, ports.Invalid("name is required")
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO targets (code, name) VALUES ($1, $2)`, item.Code, item.Name)
	if isUniqueViolation(err) {
		return nil, domain.ErrMasterExists
	}
	if err != nil {
		return nil, wrapDB("create target", err)
	}
	return &item, nil
}

func (s *CatalogStore) UpdateTargetName(code, name string) (*domain.Target, error) {
	code = domain.NormalizeCode(code)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ports.Invalid("name is required")
	}
	cmd, err := s.pool.Exec(context.Background(),
		`UPDATE targets SET name = $2 WHERE code = $1`, code, name)
	if err != nil {
		return nil, wrapDB("update target", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, domain.ErrMasterNotFound
	}
	return &domain.Target{Code: code, Name: name}, nil
}

func (s *CatalogStore) DeleteTarget(code string) error {
	code = domain.NormalizeCode(code)
	cmd, err := s.pool.Exec(context.Background(), `DELETE FROM targets WHERE code = $1`, code)
	if isFKViolation(err) {
		return domain.ErrMasterInUse
	}
	if err != nil {
		return wrapDB("delete target", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrMasterNotFound
	}
	return nil
}
