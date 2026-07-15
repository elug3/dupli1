package pg

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4/pgxpool"
)

// CatalogStore implements ports.CatalogStore against PostgreSQL.
type CatalogStore struct {
	pool *pgxpool.Pool
}

func NewCatalogStore(pool *pgxpool.Pool) *CatalogStore {
	return &CatalogStore{pool: pool}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isFKViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

func (s *CatalogStore) ListBrands() ([]domain.Brand, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT code, name FROM brands ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Brand
	for rows.Next() {
		var b domain.Brand
		if err := rows.Scan(&b.Code, &b.Name); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *CatalogStore) GetBrand(code string) (*domain.Brand, error) {
	var b domain.Brand
	err := s.pool.QueryRow(context.Background(),
		`SELECT code, name FROM brands WHERE code = $1`, domain.NormalizeCode(code),
	).Scan(&b.Code, &b.Name)
	if err != nil {
		return nil, domain.ErrMasterNotFound
	}
	return &b, nil
}

func (s *CatalogStore) CreateBrand(b domain.Brand) (*domain.Brand, error) {
	b.Code = domain.NormalizeCode(b.Code)
	b.Name = strings.TrimSpace(b.Name)
	if !domain.ValidBrandCode(b.Code) {
		return nil, fmt.Errorf("invalid brand code %q", b.Code)
	}
	if b.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO brands (code, name) VALUES ($1, $2)`, b.Code, b.Name)
	if isUniqueViolation(err) {
		return nil, domain.ErrMasterExists
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *CatalogStore) UpdateBrandName(code, name string) (*domain.Brand, error) {
	code = domain.NormalizeCode(code)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	cmd, err := s.pool.Exec(context.Background(),
		`UPDATE brands SET name = $2 WHERE code = $1`, code, name)
	if err != nil {
		return nil, err
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
		return err
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
		return nil, err
	}
	defer rows.Close()
	var out []domain.Style
	for rows.Next() {
		var st domain.Style
		if err := rows.Scan(&st.BrandCode, &st.Code, &st.Name); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s *CatalogStore) GetStyle(brandCode, code string) (*domain.Style, error) {
	var st domain.Style
	err := s.pool.QueryRow(context.Background(),
		`SELECT brand_code, code, name FROM styles WHERE brand_code = $1 AND code = $2`,
		domain.NormalizeCode(brandCode), domain.NormalizeCode(code),
	).Scan(&st.BrandCode, &st.Code, &st.Name)
	if err != nil {
		return nil, domain.ErrMasterNotFound
	}
	return &st, nil
}

func (s *CatalogStore) CreateStyle(st domain.Style) (*domain.Style, error) {
	st.BrandCode = domain.NormalizeCode(st.BrandCode)
	st.Code = domain.NormalizeCode(st.Code)
	st.Name = strings.TrimSpace(st.Name)
	if !domain.ValidBrandCode(st.BrandCode) {
		return nil, fmt.Errorf("invalid brand code %q", st.BrandCode)
	}
	if !domain.ValidSegmentCode(st.Code) {
		return nil, fmt.Errorf("invalid style code %q", st.Code)
	}
	if st.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO styles (brand_code, code, name) VALUES ($1, $2, $3)`,
		st.BrandCode, st.Code, st.Name)
	if isUniqueViolation(err) {
		return nil, domain.ErrMasterExists
	}
	if isFKViolation(err) {
		return nil, fmt.Errorf("%w: brand %s", domain.ErrMasterNotFound, st.BrandCode)
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *CatalogStore) UpdateStyleName(brandCode, code, name string) (*domain.Style, error) {
	brandCode = domain.NormalizeCode(brandCode)
	code = domain.NormalizeCode(code)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	cmd, err := s.pool.Exec(context.Background(),
		`UPDATE styles SET name = $3 WHERE brand_code = $1 AND code = $2`,
		brandCode, code, name)
	if err != nil {
		return nil, err
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
		return err
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrMasterNotFound
	}
	return nil
}

func (s *CatalogStore) ListColors() ([]domain.Color, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT code, name FROM colors ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Color
	for rows.Next() {
		var c domain.Color
		if err := rows.Scan(&c.Code, &c.Name); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *CatalogStore) GetColor(code string) (*domain.Color, error) {
	var c domain.Color
	err := s.pool.QueryRow(context.Background(),
		`SELECT code, name FROM colors WHERE code = $1`, domain.NormalizeCode(code),
	).Scan(&c.Code, &c.Name)
	if err != nil {
		return nil, domain.ErrMasterNotFound
	}
	return &c, nil
}

func (s *CatalogStore) CreateColor(c domain.Color) (*domain.Color, error) {
	c.Code = domain.NormalizeCode(c.Code)
	c.Name = strings.TrimSpace(c.Name)
	if !domain.ValidSegmentCode(c.Code) {
		return nil, fmt.Errorf("invalid color code %q", c.Code)
	}
	if c.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO colors (code, name) VALUES ($1, $2)`, c.Code, c.Name)
	if isUniqueViolation(err) {
		return nil, domain.ErrMasterExists
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *CatalogStore) UpdateColorName(code, name string) (*domain.Color, error) {
	code = domain.NormalizeCode(code)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	cmd, err := s.pool.Exec(context.Background(),
		`UPDATE colors SET name = $2 WHERE code = $1`, code, name)
	if err != nil {
		return nil, err
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
		return err
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrMasterNotFound
	}
	return nil
}

func (s *CatalogStore) ListSizes() ([]domain.Size, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT code, name FROM sizes ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Size
	for rows.Next() {
		var sz domain.Size
		if err := rows.Scan(&sz.Code, &sz.Name); err != nil {
			return nil, err
		}
		out = append(out, sz)
	}
	return out, rows.Err()
}

func (s *CatalogStore) GetSize(code string) (*domain.Size, error) {
	var sz domain.Size
	err := s.pool.QueryRow(context.Background(),
		`SELECT code, name FROM sizes WHERE code = $1`, domain.NormalizeCode(code),
	).Scan(&sz.Code, &sz.Name)
	if err != nil {
		return nil, domain.ErrMasterNotFound
	}
	return &sz, nil
}

func (s *CatalogStore) CreateSize(sz domain.Size) (*domain.Size, error) {
	sz.Code = domain.NormalizeCode(sz.Code)
	sz.Name = strings.TrimSpace(sz.Name)
	if !domain.ValidSegmentCode(sz.Code) {
		return nil, fmt.Errorf("invalid size code %q", sz.Code)
	}
	if sz.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO sizes (code, name) VALUES ($1, $2)`, sz.Code, sz.Name)
	if isUniqueViolation(err) {
		return nil, domain.ErrMasterExists
	}
	if err != nil {
		return nil, err
	}
	return &sz, nil
}

func (s *CatalogStore) UpdateSizeName(code, name string) (*domain.Size, error) {
	code = domain.NormalizeCode(code)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	cmd, err := s.pool.Exec(context.Background(),
		`UPDATE sizes SET name = $2 WHERE code = $1`, code, name)
	if err != nil {
		return nil, err
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
		return err
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrMasterNotFound
	}
	return nil
}

func (s *CatalogStore) ListEditions() ([]domain.Edition, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT code, name FROM sku_editions ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Edition
	for rows.Next() {
		var e domain.Edition
		if err := rows.Scan(&e.Code, &e.Name); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *CatalogStore) GetEdition(code string) (*domain.Edition, error) {
	var e domain.Edition
	err := s.pool.QueryRow(context.Background(),
		`SELECT code, name FROM sku_editions WHERE code = $1`, domain.NormalizeCode(code),
	).Scan(&e.Code, &e.Name)
	if err != nil {
		return nil, domain.ErrMasterNotFound
	}
	return &e, nil
}

func (s *CatalogStore) CreateEdition(e domain.Edition) (*domain.Edition, error) {
	e.Code = domain.NormalizeCode(e.Code)
	e.Name = strings.TrimSpace(e.Name)
	if !domain.ValidSegmentCode(e.Code) {
		return nil, fmt.Errorf("invalid edition code %q", e.Code)
	}
	if e.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO sku_editions (code, name) VALUES ($1, $2)`, e.Code, e.Name)
	if isUniqueViolation(err) {
		return nil, domain.ErrMasterExists
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *CatalogStore) UpdateEditionName(code, name string) (*domain.Edition, error) {
	code = domain.NormalizeCode(code)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	cmd, err := s.pool.Exec(context.Background(),
		`UPDATE sku_editions SET name = $2 WHERE code = $1`, code, name)
	if err != nil {
		return nil, err
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
		return err
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrMasterNotFound
	}
	return nil
}
