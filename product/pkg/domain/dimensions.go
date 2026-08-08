package domain

import "fmt"

// MaxDimensionMm is the upper bound for a single axis (10 m). Far above any
// bag/wallet/clothing size we sell; catches unit mistakes (cm sent as mm ×10).
const MaxDimensionMm = 10000

// Dimensions is the physical size of a sellable SKU in millimeters.
// Distinct from Size / SizeCode (letter labels like S/M/L used in the human SKU).
//
// Unit is always millimeters. Omit any axis that does not apply (e.g. flat
// wallets may leave depth at 0 / omitted).
type Dimensions struct {
	WidthMm  int `json:"widthMm,omitempty"`
	HeightMm int `json:"heightMm,omitempty"`
	DepthMm  int `json:"depthMm,omitempty"`
}

// Empty reports whether every axis is unset.
func (d Dimensions) Empty() bool {
	return d.WidthMm == 0 && d.HeightMm == 0 && d.DepthMm == 0
}

// NormalizeDimensions validates and canonicalizes dimensions on write.
//
//	nil  → nil (field omitted / cleared after empty)
//	{}   → nil (explicit clear)
//	values must be non-negative and ≤ MaxDimensionMm
func NormalizeDimensions(d *Dimensions) (*Dimensions, error) {
	if d == nil || d.Empty() {
		return nil, nil
	}
	if d.WidthMm < 0 || d.HeightMm < 0 || d.DepthMm < 0 {
		return nil, fmt.Errorf("dimensions must be non-negative millimeters")
	}
	if d.WidthMm > MaxDimensionMm || d.HeightMm > MaxDimensionMm || d.DepthMm > MaxDimensionMm {
		return nil, fmt.Errorf("dimensions must be at most %d mm", MaxDimensionMm)
	}
	out := *d
	return &out, nil
}
