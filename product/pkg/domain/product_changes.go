package domain

import (
	"sort"
	"strconv"
	"strings"
)

// FieldChange is one parent field an update changed, with its value before
// and after as text. Prices are whole won written as digits. Field is the
// product's JSON field name, or "attributes.<key>" for one attribute.
type FieldChange struct {
	Field  string
	Before string
	After  string
}

// ProductChanges lists the manager-editable parent fields that differ between
// before and after, in a fixed order, so an ops alert can say what an edit
// changed instead of only where the product ended up. Derived fields (counts,
// images, variants, timestamps) are left out: they are not what a product
// edit changes.
func ProductChanges(before, after Product) []FieldChange {
	var out []FieldChange
	add := func(field, b, a string) {
		if b != a {
			out = append(out, FieldChange{Field: field, Before: b, After: a})
		}
	}
	add("name", before.Name, after.Name)
	add("status", before.Status, after.Status)
	add("price", wonText(before.Price), wonText(after.Price))
	add("officialPrice", wonText(before.OfficialPrice), wonText(after.OfficialPrice))
	add("brand", before.Brand, after.Brand)
	add("brandCode", before.BrandCode, after.BrandCode)
	add("styleCode", before.StyleCode, after.StyleCode)
	add("category", before.Category, after.Category)
	add("subCategory", before.SubCategory, after.SubCategory)
	add("style", before.Style, after.Style)
	add("target", before.Target, after.Target)
	add("material", before.Material, after.Material)
	add("capacity", before.Capacity, after.Capacity)
	add("description", before.Description, after.Description)
	add("tags", strings.Join(before.Tags, ", "), strings.Join(after.Tags, ", "))
	add("sizeChart", sizeChartText(before.SizeChart), sizeChartText(after.SizeChart))

	keys := map[string]struct{}{}
	for k := range before.Attributes {
		keys[k] = struct{}{}
	}
	for k := range after.Attributes {
		keys[k] = struct{}{}
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		add("attributes."+k, before.Attributes[k], after.Attributes[k])
	}
	return out
}

// wonText writes a price as whole won digits; an unset price is empty.
func wonText(v float64) string {
	if v == 0 {
		return ""
	}
	return strconv.FormatInt(int64(v), 10)
}

// sizeChartText renders the chart one row per size, "M 52/70/45/62"
// (chest/length/shoulder/sleeve cm, "-" for an unmeasured one), so a changed
// measurement shows up as a changed row.
func sizeChartText(rows []SizeChartRow) string {
	parts := make([]string, 0, len(rows))
	for _, r := range rows {
		parts = append(parts, r.Size+" "+cmText(r.ChestCm)+"/"+cmText(r.LengthCm)+"/"+cmText(r.ShoulderCm)+"/"+cmText(r.SleeveCm))
	}
	return strings.Join(parts, ", ")
}

func cmText(v float64) string {
	if v == 0 {
		return "-"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}
