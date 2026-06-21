package pg

import (
	"context"
	"fmt"

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

func (s *ProductSearchStore) SearchConsultations(filter map[string]string) ([]domain.Consultation, error) {
	query := "SELECT id, title, description, duration, price, status FROM consultations WHERE 1=1"
	args := []interface{}{}
	idx := 1

	for key, value := range filter {
		switch key {
		case "title":
			query += fmt.Sprintf(" AND title ILIKE $%d", idx)
			args = append(args, "%"+value+"%")
			idx++
		case "status":
			query += fmt.Sprintf(" AND status = $%d", idx)
			args = append(args, value)
			idx++
		}
	}

	rows, err := s.pool.Query(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []domain.Consultation
	for rows.Next() {
		var c domain.Consultation
		if err := rows.Scan(&c.ID, &c.Title, &c.Description, &c.Duration, &c.Price, &c.Status); err != nil {
			return nil, err
		}
		results = append(results, c)
	}

	return results, rows.Err()
}

func (s *ProductSearchStore) SearchShoes(filter map[string]string) ([]domain.Shoes, error) {
	query := "SELECT id, name, description, price, first_price, brand, size, color, gender, material, stock, category FROM shoes WHERE 1=1"
	args := []interface{}{}
	idx := 1

	for key, value := range filter {
		switch key {
		case "brand":
			query += fmt.Sprintf(" AND brand ILIKE $%d", idx)
			args = append(args, "%"+value+"%")
			idx++
		case "size":
			query += fmt.Sprintf(" AND size = $%d", idx)
			args = append(args, value)
			idx++
		case "color":
			query += fmt.Sprintf(" AND color = $%d", idx)
			args = append(args, value)
			idx++
		case "gender":
			query += fmt.Sprintf(" AND gender = $%d", idx)
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

	var results []domain.Shoes
	for rows.Next() {
		var shoe domain.Shoes
		if err := rows.Scan(&shoe.ID, &shoe.Name, &shoe.Description, &shoe.Price, &shoe.FirstPrice, &shoe.Brand, &shoe.Size, &shoe.Color, &shoe.Gender, &shoe.Material, &shoe.Stock, &shoe.Category); err != nil {
			return nil, err
		}
		results = append(results, shoe)
	}

	return results, rows.Err()
}

func (s *ProductSearchStore) SearchOuterwear(filter map[string]string) ([]domain.Outerwear, error) {
	query := "SELECT id, name, description, price, first_price, brand, size, color, gender, material, stock, category FROM outerwear WHERE 1=1"
	args := []interface{}{}
	idx := 1

	for key, value := range filter {
		switch key {
		case "brand":
			query += fmt.Sprintf(" AND brand ILIKE $%d", idx)
			args = append(args, "%"+value+"%")
			idx++
		case "size":
			query += fmt.Sprintf(" AND size = $%d", idx)
			args = append(args, value)
			idx++
		case "color":
			query += fmt.Sprintf(" AND color = $%d", idx)
			args = append(args, value)
			idx++
		case "gender":
			query += fmt.Sprintf(" AND gender = $%d", idx)
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

	var results []domain.Outerwear
	for rows.Next() {
		var item domain.Outerwear
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Price, &item.FirstPrice, &item.Brand, &item.Size, &item.Color, &item.Gender, &item.Material, &item.Stock, &item.Category); err != nil {
			return nil, err
		}
		results = append(results, item)
	}

	return results, rows.Err()
}

func (s *ProductSearchStore) SearchBottoms(filter map[string]string) ([]domain.Bottoms, error) {
	query := "SELECT id, name, description, price, first_price, brand, size, color, gender, material, stock, category FROM bottoms WHERE 1=1"
	args := []interface{}{}
	idx := 1

	for key, value := range filter {
		switch key {
		case "brand":
			query += fmt.Sprintf(" AND brand ILIKE $%d", idx)
			args = append(args, "%"+value+"%")
			idx++
		case "size":
			query += fmt.Sprintf(" AND size = $%d", idx)
			args = append(args, value)
			idx++
		case "color":
			query += fmt.Sprintf(" AND color = $%d", idx)
			args = append(args, value)
			idx++
		case "gender":
			query += fmt.Sprintf(" AND gender = $%d", idx)
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

	var results []domain.Bottoms
	for rows.Next() {
		var item domain.Bottoms
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Price, &item.FirstPrice, &item.Brand, &item.Size, &item.Color, &item.Gender, &item.Material, &item.Stock, &item.Category); err != nil {
			return nil, err
		}
		results = append(results, item)
	}

	return results, rows.Err()
}

func (s *ProductSearchStore) SearchBags(filter map[string]string) ([]domain.Bag, error) {
	query := "SELECT id, name, description, price, first_price, brand, color, material, capacity, stock, category FROM bags WHERE 1=1"
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
		if err := rows.Scan(&b.ID, &b.Name, &b.Description, &b.Price, &b.FirstPrice, &b.Brand, &b.Color, &b.Material, &b.Capacity, &b.Stock, &b.Category); err != nil {
			return nil, err
		}
		results = append(results, b)
	}

	return results, rows.Err()
}

func (s *ProductSearchStore) SearchClocks(filter map[string]string) ([]domain.Clock, error) {
	query := "SELECT id, name, description, price, first_price, brand, type, material, stock, category FROM clocks WHERE 1=1"
	args := []interface{}{}
	idx := 1

	for key, value := range filter {
		switch key {
		case "brand":
			query += fmt.Sprintf(" AND brand ILIKE $%d", idx)
			args = append(args, "%"+value+"%")
			idx++
		case "type":
			query += fmt.Sprintf(" AND type = $%d", idx)
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

	var results []domain.Clock
	for rows.Next() {
		var c domain.Clock
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.Price, &c.FirstPrice, &c.Brand, &c.Type, &c.Material, &c.Stock, &c.Category); err != nil {
			return nil, err
		}
		results = append(results, c)
	}

	return results, rows.Err()
}
