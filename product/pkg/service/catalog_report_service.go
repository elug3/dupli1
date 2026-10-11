package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

// ErrInvalidProductStatus is returned for a status filter that is not a
// product status.
var ErrInvalidProductStatus = errors.New("status must be active, draft or archived")

var productStatuses = []string{"active", "draft", "archived"}

// CatalogReportService answers how many products, SKUs and units each brand
// and category holds, for the manage console's catalog overview.
type CatalogReportService struct {
	store   ports.CatalogReportStore
	masters ports.CatalogStore
}

func NewCatalogReportService(store ports.CatalogReportStore, masters ports.CatalogStore) *CatalogReportService {
	return &CatalogReportService{store: store, masters: masters}
}

// Report counts the catalog, narrowed to products of status when it is set.
// Every brand master and every taxonomy category and subcategory is listed,
// with zeros where nothing matches, so an empty brand is visible rather than
// missing.
func (s *CatalogReportService) Report(ctx context.Context, status string) (domain.CatalogReport, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "" && !containsString(productStatuses, status) {
		return domain.CatalogReport{}, ErrInvalidProductStatus
	}
	counts, err := s.store.CatalogCounts(ctx)
	if err != nil {
		return domain.CatalogReport{}, err
	}
	masters, err := s.masters.ListBrands(ctx)
	if err != nil {
		return domain.CatalogReport{}, err
	}

	report := domain.CatalogReport{
		Status:           status,
		ProductsByStatus: map[string]int{},
		Brands:           []domain.CatalogBrandCount{},
		Categories:       []domain.CatalogCategoryCount{},
		Cells:            []domain.CatalogCell{},
	}
	for _, st := range productStatuses {
		report.ProductsByStatus[st] = 0
	}

	brands := map[string]*domain.CatalogBrandCount{}
	var brandOrder []string
	brandFor := func(code, name string) *domain.CatalogBrandCount {
		key := brandKey(code, name)
		if b, ok := brands[key]; ok {
			return b
		}
		b := &domain.CatalogBrandCount{Code: code, Name: name}
		brands[key] = b
		brandOrder = append(brandOrder, key)
		return b
	}
	masterNames := map[string]string{}
	for _, m := range masters {
		masterNames[m.Code] = m.Name
		brandFor(m.Code, m.Name)
	}

	categories, findSub := newCategoryTally()
	cells := map[[3]string]*domain.CatalogCell{}
	var cellOrder [][3]string

	for _, c := range counts {
		report.ProductsByStatus[c.Status] += c.Products
		if status != "" && c.Status != status {
			continue
		}
		report.Totals.Add(c)

		name := c.Brand
		if n, ok := masterNames[c.BrandCode]; ok && c.BrandCode != "" {
			name = n
		}
		if name == "" {
			name = c.BrandCode
		}
		b := brandFor(c.BrandCode, name)
		b.Add(c)

		cat, sub := findSub(c.Category, c.SubCategory)
		cat.Add(c)
		sub.Add(c)

		key := [3]string{brandKey(c.BrandCode, name), cat.Code, sub.Code}
		cell, ok := cells[key]
		if !ok {
			cell = &domain.CatalogCell{BrandCode: c.BrandCode, Brand: name, Category: cat.Code, SubCategory: sub.Code}
			cells[key] = cell
			cellOrder = append(cellOrder, key)
		}
		cell.Add(c)
	}

	for _, key := range brandOrder {
		report.Brands = append(report.Brands, *brands[key])
	}
	sort.SliceStable(report.Brands, func(i, j int) bool {
		a, b := report.Brands[i], report.Brands[j]
		if a.Products != b.Products {
			return a.Products > b.Products
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	report.Categories = categories()
	for _, key := range cellOrder {
		if cell := cells[key]; cell.Products > 0 || cell.SKUs > 0 {
			report.Cells = append(report.Cells, *cell)
		}
	}
	return report, nil
}

// brandKey groups by brand master code, or by name for a product that names
// a brand without one.
func brandKey(code, name string) string {
	if code != "" {
		return "code:" + code
	}
	return "name:" + strings.ToLower(strings.TrimSpace(name))
}

// newCategoryTally seeds every taxonomy category and subcategory with zeros
// and returns a lookup that adds unknown ones (and the empty "no
// subcategory" bucket) as products turn up in them, plus a function that
// returns the result in taxonomy order. Empty buckets the taxonomy does not
// name are dropped.
func newCategoryTally() (func() []domain.CatalogCategoryCount, func(category, sub string) (*domain.CatalogCategoryCount, *domain.CatalogSubCategoryCount)) {
	type entry struct {
		cat  *domain.CatalogCategoryCount
		subs map[string]*domain.CatalogSubCategoryCount
		// order lists sub codes; seeded ones first.
		order  []string
		seeded map[string]bool
	}
	var order []string
	byCode := map[string]*entry{}
	addCategory := func(code, name string) *entry {
		e := &entry{
			cat:    &domain.CatalogCategoryCount{Code: code, Name: name},
			subs:   map[string]*domain.CatalogSubCategoryCount{},
			seeded: map[string]bool{},
		}
		byCode[code] = e
		order = append(order, code)
		return e
	}
	addSub := func(e *entry, code, name string) *domain.CatalogSubCategoryCount {
		s := &domain.CatalogSubCategoryCount{Code: code, Name: name}
		e.subs[code] = s
		e.order = append(e.order, code)
		return s
	}
	for _, c := range domain.Categories() {
		e := addCategory(c.Code, c.Name)
		for _, sc := range c.SubCategories {
			addSub(e, sc.Code, sc.Name)
			e.seeded[sc.Code] = true
		}
	}

	find := func(category, sub string) (*domain.CatalogCategoryCount, *domain.CatalogSubCategoryCount) {
		category = strings.ToLower(strings.TrimSpace(category))
		sub = strings.ToLower(strings.TrimSpace(sub))
		e, ok := byCode[category]
		if !ok {
			e = addCategory(category, category)
		}
		s, ok := e.subs[sub]
		if !ok {
			s = addSub(e, sub, sub)
		}
		return e.cat, s
	}
	result := func() []domain.CatalogCategoryCount {
		out := make([]domain.CatalogCategoryCount, 0, len(order))
		for _, code := range order {
			e := byCode[code]
			cat := *e.cat
			cat.SubCategories = []domain.CatalogSubCategoryCount{}
			for _, sc := range e.order {
				s := e.subs[sc]
				if !e.seeded[sc] && s.Products == 0 && s.SKUs == 0 {
					continue
				}
				cat.SubCategories = append(cat.SubCategories, *s)
			}
			out = append(out, cat)
		}
		return out
	}
	return result, find
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
