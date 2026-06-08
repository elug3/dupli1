package pg

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/schick/pkg/product/domain"
)

type ProductSearchStore struct {
	pool *pgxpool.Pool
}

func NewProductStore(connString string) (*ProductSearchStore, error) {
	pool, err := pgxpool.Connect(context.Background(), connString)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &ProductSearchStore{pool: pool}, nil
}

func (s *ProductSearchStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *ProductSearchStore) SearchBags(filter map[string]string) ([]domain.Bag, error) {
	query := "SELECT id, name, description, price, brand, color, material, capacity, stock FROM bags WHERE 1=1"
	args := []interface{}{}
	idx := 1

	for key, value := range filter {
		switch key {
		case "brand":
			query += fmt.Sprintf(" AND brand ILIKE $%d", idx)
			args = append(args, "%"+value+"%")
			idx++
		case "color":
			query += fmt.Sprintf(" AND color = $%d", idx)
			args = append(args, value)
			idx++
		case "material":
			query += fmt.Sprintf(" AND material = $%d", idx)
			args = append(args, value)
			idx++
		}
	}

	rows, err := s.pool.Query(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []domain.Bag
	for rows.Next() {
		var b domain.Bag
		if err := rows.Scan(&b.ID, &b.Name, &b.Description, &b.Price, &b.Brand, &b.Color, &b.Material, &b.Capacity, &b.Stock); err != nil {
			return nil, err
		}
		results = append(results, b)
	}

	return results, rows.Err()
}

func (s *ProductSearchStore) CreateProduct(p domain.Product) (*domain.Product, error) {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO products (id, name, description, price, brand, color, material, stock)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		p.ID, p.Name, p.Description, p.Price, p.Brand, p.Color, p.Material, p.Stock,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
