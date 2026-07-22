package memory

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

// ProductStore is an in-memory implementation of ports.ProductStore, for use in tests.
type ProductStore struct {
	Products []domain.Product
	Variants []domain.Variant
	Catalog  *CatalogStore
	// views keys are guestID + "\x00" + productID for unique PDP views.
	views map[string]struct{}
	// wishlists keys are ownerKey + "\x00" + productID; createdAt tracks insert order.
	wishlists map[string]time.Time
}

func NewProductStore() *ProductStore {
	return &ProductStore{Catalog: NewCatalogStore()}
}

func (s *ProductStore) catalog() *CatalogStore {
	if s.Catalog == nil {
		s.Catalog = NewCatalogStore()
	}
	return s.Catalog
}

func (s *ProductStore) variantsFor(productID string) []domain.Variant {
	var out []domain.Variant
	for _, v := range s.Variants {
		if v.ProductID == productID {
			out = append(out, v)
		}
	}
	return out
}

func (s *ProductStore) enrichMasterNames(products []domain.Product) {
	cat := s.catalog()
	for i := range products {
		p := &products[i]
		if p.Brand == "" && p.BrandCode != "" {
			if b, err := cat.GetBrand(p.BrandCode); err == nil {
				p.Brand = b.Name
			}
		}
		for j := range p.Variants {
			v := &p.Variants[j]
			if v.Color == "" && v.ColorCode != "" {
				if c, err := cat.GetColor(v.ColorCode); err == nil {
					v.Color = c.Name
				}
			}
			if v.Size == "" && v.SizeCode != "" {
				if sz, err := cat.GetSize(v.SizeCode); err == nil {
					v.Size = sz.Name
				}
			}
		}
	}
}

func (s *ProductStore) enrich(products []domain.Product, includeVariants bool) {
	for i := range products {
		products[i].EnrichFromVariants(s.variantsFor(products[i].ID), includeVariants)
	}
	s.enrichMasterNames(products)
}

func (s *ProductStore) SearchProducts(filter map[string]string) ([]domain.Product, int, error) {
	var results []domain.Product
	q := strings.ToLower(strings.TrimSpace(filter["q"]))
	for _, p := range s.Products {
		if q != "" {
			hay := strings.ToLower(p.Name + " " + p.Brand + " " + p.Description)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		if category := filter["category"]; category != "" && p.Category != category {
			continue
		}
		if status := filter["status"]; status != "" && p.Status != status {
			continue
		}
		if brand := filter["brand"]; brand != "" && !strings.Contains(strings.ToLower(p.Brand), strings.ToLower(brand)) {
			continue
		}
		if material := filter["material"]; material != "" && p.Material != material {
			continue
		}
		if tags := filter["tags"]; tags != "" && !hasAllTags(p.Tags, tags) {
			continue
		}
		variants := s.variantsFor(p.ID)
		if color := filter["color"]; color != "" && !hasActiveOption(variants, "color", color) {
			continue
		}
		if size := filter["size"]; size != "" && !hasActiveOption(variants, "size", size) {
			continue
		}
		if after := filter["created_after"]; after != "" {
			cutoff, err := time.Parse(time.RFC3339, after)
			if err != nil {
				continue
			}
			created, err := time.Parse(time.RFC3339, p.CreatedAt)
			if err != nil || created.Before(cutoff) {
				continue
			}
		}
		results = append(results, p)
	}

	// Enrich before sort so price_from is available for sort=price.
	s.enrich(results, false)
	sortProducts(results, filter)

	total := len(results)
	if limit, ok := atoiFilter(filter, "limit"); ok && limit > 0 {
		offset, _ := atoiFilter(filter, "offset")
		if offset < 0 {
			offset = 0
		}
		if offset >= len(results) {
			results = nil
		} else {
			end := offset + limit
			if end > len(results) {
				end = len(results)
			}
			results = results[offset:end]
		}
	}
	return results, total, nil
}

func sortProducts(results []domain.Product, filter map[string]string) {
	sortKey := domain.NormalizeSearchSort(filter["sort"])
	if sortKey == "" {
		sortKey = domain.SortNewest
	}
	order := domain.NormalizeSearchOrder(filter["order"], sortKey)
	if order == "" {
		order = domain.OrderDesc
	}
	asc := order == domain.OrderAsc

	sort.SliceStable(results, func(i, j int) bool {
		a, b := results[i], results[j]
		var less bool
		switch sortKey {
		case domain.SortViews:
			less = a.ViewCount < b.ViewCount
		case domain.SortSold:
			less = a.SoldCount < b.SoldCount
		case domain.SortWishlist:
			less = a.WishlistCount < b.WishlistCount
		case domain.SortPrice:
			less = a.PriceFrom < b.PriceFrom
		case domain.SortName:
			less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
		default:
			less = a.CreatedAt < b.CreatedAt
		}
		if aEqual(sortKey, a, b) {
			return a.ID < b.ID
		}
		if asc {
			return less
		}
		return !less
	})
}

func aEqual(sortKey string, a, b domain.Product) bool {
	switch sortKey {
	case domain.SortViews:
		return a.ViewCount == b.ViewCount
	case domain.SortSold:
		return a.SoldCount == b.SoldCount
	case domain.SortWishlist:
		return a.WishlistCount == b.WishlistCount
	case domain.SortPrice:
		return a.PriceFrom == b.PriceFrom
	case domain.SortName:
		return strings.EqualFold(a.Name, b.Name)
	default:
		return a.CreatedAt == b.CreatedAt
	}
}

func atoiFilter(filter map[string]string, key string) (int, bool) {
	raw, ok := filter[key]
	if !ok || raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return n, true
}

func hasActiveOption(variants []domain.Variant, field, want string) bool {
	for _, v := range variants {
		if v.Status != "" && v.Status != "active" {
			continue
		}
		switch field {
		case "color":
			if v.Color == want {
				return true
			}
		case "size":
			if v.Size == want {
				return true
			}
		}
	}
	return false
}

func hasAllTags(have []string, wantCSV string) bool {
	for _, want := range strings.Split(wantCSV, ",") {
		want = strings.TrimSpace(want)
		if want == "" {
			continue
		}
		found := false
		for _, tag := range have {
			if tag == want {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (s *ProductStore) ListProducts() ([]domain.Product, error) {
	results, _, err := s.SearchProducts(nil)
	return results, err
}

func (s *ProductStore) GetProduct(id string) (*domain.Product, error) {
	for _, p := range s.Products {
		if p.ID == id {
			out := p
			out.EnrichFromVariants(s.variantsFor(id), true)
			return &out, nil
		}
	}
	return nil, fmt.Errorf("product %s: %w", id, ports.ErrNotFound)
}

func (s *ProductStore) GetActiveProduct(id string) (*domain.Product, error) {
	p, err := s.GetProduct(id)
	if err != nil {
		return nil, err
	}
	if p.Status != "active" {
		return nil, fmt.Errorf("product %s: %w", id, ports.ErrNotFound)
	}
	active := make([]domain.Variant, 0, len(p.Variants))
	for _, v := range p.Variants {
		if v.Status == "active" {
			active = append(active, v)
		}
	}
	p.EnrichFromVariants(active, true)
	return p, nil
}

func brandPrefix(brand string) string {
	if code := domain.BrandCodeFromName(brand); code != "" {
		return code
	}
	return "PRD"
}

func (s *ProductStore) nextProductID(brand string) string {
	prefix := brandPrefix(brand)
	max := 0
	for _, p := range s.Products {
		if strings.HasPrefix(p.ID, prefix+"-") {
			if n, err := strconv.Atoi(p.ID[len(prefix)+1:]); err == nil && n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("%s-%03d", prefix, max+1)
}

func (s *ProductStore) CreateProduct(p domain.Product) (*domain.Product, error) {
	if p.ID == "" {
		p.ID = domain.NewProductID()
	}
	if p.Status == "" {
		p.Status = "active"
	}
	if err := domain.RequireProductSKUCodes(&p); err != nil {
		return nil, err
	}
	cat := s.catalog()
	if _, err := cat.GetBrand(p.BrandCode); err != nil {
		return nil, fmt.Errorf("%w: brand %s", domain.ErrMasterNotFound, p.BrandCode)
	}
	if _, err := cat.GetStyle(p.BrandCode, p.StyleCode); err != nil {
		return nil, fmt.Errorf("%w: style %s/%s", domain.ErrMasterNotFound, p.BrandCode, p.StyleCode)
	}
	if p.Brand == "" {
		if b, err := cat.GetBrand(p.BrandCode); err == nil {
			p.Brand = b.Name
		}
	}
	s.Products = append(s.Products, p)
	cat.ProductBrandStyles[p.ID] = p.BrandCode + "|" + p.StyleCode

	switch {
	case len(p.Variants) > 0:
		for _, v := range p.Variants {
			v.ProductID = p.ID
			if v.Status == "" {
				v.Status = p.Status
			}
			if _, err := s.CreateVariant(v); err != nil {
				return nil, err
			}
		}
	case p.Color != "" || p.Price > 0 || p.SellingPrice > 0 || len(p.ImageURLs) > 0:
		if _, err := s.CreateVariant(domain.Variant{
			ProductID:    p.ID,
			Color:        p.Color,
			SellingPrice: p.SellingPrice,
			Price:        p.Price,
			Status:       p.Status,
			ImageURLs:    p.ImageURLs,
		}); err != nil {
			return nil, err
		}
	}

	return s.GetProduct(p.ID)
}

func (s *ProductStore) UpdateProduct(p domain.Product) (*domain.Product, error) {
	for i, existing := range s.Products {
		if existing.ID == p.ID {
			p.CreatedAt = existing.CreatedAt
			p.CreatedBy = existing.CreatedBy
			// Brand/style codes are immutable after creation.
			p.BrandCode = existing.BrandCode
			p.StyleCode = existing.StyleCode
			s.Products[i] = p
			return s.GetProduct(p.ID)
		}
	}
	return nil, fmt.Errorf("product %s: %w", p.ID, ports.ErrNotFound)
}

func (s *ProductStore) DeleteProduct(id string) error {
	for i, p := range s.Products {
		if p.ID == id {
			s.Products = append(s.Products[:i], s.Products[i+1:]...)
			kept := s.Variants[:0]
			for _, v := range s.Variants {
				if v.ProductID != id {
					kept = append(kept, v)
				}
			}
			s.Variants = kept
			prefix := "\x00" + id
			for k := range s.views {
				if strings.HasSuffix(k, prefix) {
					delete(s.views, k)
				}
			}
			for k := range s.wishlists {
				if strings.HasSuffix(k, prefix) {
					delete(s.wishlists, k)
				}
			}
			return nil
		}
	}
	return fmt.Errorf("product %s: %w", id, ports.ErrNotFound)
}

// RecordUniqueView implements ports.ProductViewStore.
func (s *ProductStore) RecordUniqueView(guestID, productID string) (bool, int64, error) {
	if guestID == "" || productID == "" {
		return false, 0, fmt.Errorf("guest id and product id are required")
	}
	if s.views == nil {
		s.views = make(map[string]struct{})
	}
	key := guestID + "\x00" + productID
	for i := range s.Products {
		if s.Products[i].ID != productID {
			continue
		}
		if _, ok := s.views[key]; ok {
			return false, s.Products[i].ViewCount, nil
		}
		s.views[key] = struct{}{}
		s.Products[i].ViewCount++
		return true, s.Products[i].ViewCount, nil
	}
	return false, 0, fmt.Errorf("product %s: %w", productID, ports.ErrNotFound)
}

// AddWishlist implements ports.ProductWishlistStore.
func (s *ProductStore) AddWishlist(ownerKey, productID string) (bool, int64, error) {
	if ownerKey == "" || productID == "" {
		return false, 0, fmt.Errorf("owner key and product id are required")
	}
	if s.wishlists == nil {
		s.wishlists = make(map[string]time.Time)
	}
	key := ownerKey + "\x00" + productID
	for i := range s.Products {
		if s.Products[i].ID != productID {
			continue
		}
		if _, ok := s.wishlists[key]; ok {
			return false, s.Products[i].WishlistCount, nil
		}
		s.wishlists[key] = time.Now().UTC()
		s.Products[i].WishlistCount++
		return true, s.Products[i].WishlistCount, nil
	}
	return false, 0, fmt.Errorf("product %s: %w", productID, ports.ErrNotFound)
}

// RemoveWishlist implements ports.ProductWishlistStore.
func (s *ProductStore) RemoveWishlist(ownerKey, productID string) (bool, int64, error) {
	if ownerKey == "" || productID == "" {
		return false, 0, fmt.Errorf("owner key and product id are required")
	}
	if s.wishlists == nil {
		s.wishlists = make(map[string]time.Time)
	}
	key := ownerKey + "\x00" + productID
	for i := range s.Products {
		if s.Products[i].ID != productID {
			continue
		}
		if _, ok := s.wishlists[key]; !ok {
			return false, s.Products[i].WishlistCount, nil
		}
		delete(s.wishlists, key)
		if s.Products[i].WishlistCount > 0 {
			s.Products[i].WishlistCount--
		}
		return true, s.Products[i].WishlistCount, nil
	}
	return false, 0, nil
}

// ListWishlistProductIDs implements ports.ProductWishlistStore.
func (s *ProductStore) ListWishlistProductIDs(ownerKey string) ([]string, error) {
	type entry struct {
		id string
		at time.Time
	}
	var entries []entry
	prefix := ownerKey + "\x00"
	for k, at := range s.wishlists {
		if strings.HasPrefix(k, prefix) {
			entries = append(entries, entry{id: strings.TrimPrefix(k, prefix), at: at})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].at.Equal(entries[j].at) {
			return entries[i].at.After(entries[j].at)
		}
		return entries[i].id < entries[j].id
	})
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.id
	}
	return ids, nil
}

func (s *ProductStore) nextVariantSKU(productID, brandCode, styleCode string, v *domain.Variant) (string, error) {
	base := domain.ComposeVariantSKU(productID, brandCode, styleCode, v)
	candidate := base
	for i := 1; ; i++ {
		exists := false
		for _, existing := range s.Variants {
			if existing.SKU == candidate {
				exists = true
				break
			}
		}
		if !exists {
			return candidate, nil
		}
		if brandCode != "" && styleCode != "" && v != nil && v.ColorCode != "" {
			return "", ports.Conflict(fmt.Sprintf("duplicate sku %s: same brand/style/color/edition/size already exists", base))
		}
		candidate = fmt.Sprintf("%s-%d", base, i+1)
	}
}

func (s *ProductStore) ListVariants(productID string) ([]domain.Variant, error) {
	return s.variantsFor(productID), nil
}

func (s *ProductStore) GetVariant(sku string) (*domain.Variant, error) {
	for _, v := range s.Variants {
		if v.SKU == sku {
			out := v
			return &out, nil
		}
	}
	return nil, fmt.Errorf("variant %s: %w", sku, ports.ErrNotFound)
}

func (s *ProductStore) GetVariantBySkuID(skuID string) (*domain.Variant, error) {
	for _, v := range s.Variants {
		if v.SkuID == skuID {
			out := v
			return &out, nil
		}
	}
	return nil, fmt.Errorf("variant %s: %w", skuID, ports.ErrNotFound)
}

func (s *ProductStore) GetVariantsBySkuIDs(skuIDs []string) ([]domain.Variant, error) {
	if len(skuIDs) == 0 {
		return nil, nil
	}
	want := make(map[string]struct{}, len(skuIDs))
	for _, id := range skuIDs {
		want[id] = struct{}{}
	}
	var results []domain.Variant
	for _, v := range s.Variants {
		if _, ok := want[v.SkuID]; ok {
			out := v
			results = append(results, out)
		}
	}
	return results, nil
}

func (s *ProductStore) CreateVariant(v domain.Variant) (*domain.Variant, error) {
	if v.ProductID == "" {
		return nil, ports.Invalid("productId is required")
	}
	var brandCode, styleCode string
	found := false
	for _, p := range s.Products {
		if p.ID == v.ProductID {
			found = true
			brandCode, styleCode = p.BrandCode, p.StyleCode
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("product %s: %w", v.ProductID, ports.ErrNotFound)
	}
	if brandCode == "" || styleCode == "" {
		return nil, fmt.Errorf("%w: parent product missing brandCode/styleCode", domain.ErrMissingSKUCodes)
	}
	if v.Status == "" {
		v.Status = "active"
	}
	if err := domain.RequireVariantSKUCodes(&v); err != nil {
		return nil, err
	}
	cat := s.catalog()
	if _, err := cat.GetColor(v.ColorCode); err != nil {
		return nil, fmt.Errorf("%w: color %s", domain.ErrMasterNotFound, v.ColorCode)
	}
	if _, err := cat.GetSize(v.SizeCode); err != nil {
		return nil, fmt.Errorf("%w: size %s", domain.ErrMasterNotFound, v.SizeCode)
	}
	if v.EditionCode != "" {
		if _, err := cat.GetEdition(v.EditionCode); err != nil {
			return nil, fmt.Errorf("%w: edition %s", domain.ErrMasterNotFound, v.EditionCode)
		}
	}
	if v.Color == "" {
		if c, err := cat.GetColor(v.ColorCode); err == nil {
			v.Color = c.Name
		}
	}
	if v.Size == "" {
		if sz, err := cat.GetSize(v.SizeCode); err == nil {
			v.Size = sz.Name
		}
	}
	if v.SKU == "" {
		sku, err := s.nextVariantSKU(v.ProductID, brandCode, styleCode, &v)
		if err != nil {
			return nil, err
		}
		v.SKU = sku
	}
	if v.SkuID == "" {
		v.SkuID = domain.NewSkuID()
	}
	for _, existing := range s.Variants {
		if existing.SKU == v.SKU {
			return nil, ports.Conflict(fmt.Sprintf("variant already exists: %s", v.SKU))
		}
		if existing.SkuID == v.SkuID {
			return nil, ports.Conflict(fmt.Sprintf("variant already exists: %s", v.SkuID))
		}
		if existing.ProductID == v.ProductID && existing.Color == v.Color && existing.Size == v.Size {
			return nil, ports.Conflict("variant option already exists")
		}
	}
	s.Variants = append(s.Variants, v)
	cat.VariantColorCodes[v.SKU] = v.ColorCode
	cat.VariantSizeCodes[v.SKU] = v.SizeCode
	if v.EditionCode != "" {
		cat.VariantEditionCodes[v.SKU] = v.EditionCode
	}
	return &v, nil
}

// UpdateVariant updates a variant by its (immutable) sku. SkuID is always
// preserved from the existing row regardless of what the caller passed in.
func (s *ProductStore) UpdateVariant(v domain.Variant) (*domain.Variant, error) {
	for i, existing := range s.Variants {
		if existing.SKU == v.SKU {
			v.SkuID = existing.SkuID
			v.ProductID = existing.ProductID
			v.CreatedAt = existing.CreatedAt
			s.Variants[i] = v
			return &v, nil
		}
	}
	return nil, fmt.Errorf("variant %s: %w", v.SKU, ports.ErrNotFound)
}

func (s *ProductStore) DeleteVariant(sku string) error {
	for i, v := range s.Variants {
		if v.SKU == sku {
			s.Variants = append(s.Variants[:i], s.Variants[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("variant %s: %w", sku, ports.ErrNotFound)
}
