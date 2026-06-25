package domain

type Coupon struct {
	Code        string  `json:"code"`
	Discount    float64 `json:"discount"`    // fraction, e.g. 0.30 for 30 %
	Description string  `json:"description"`
	Expires     string  `json:"expires"`
	Active      bool    `json:"active"`
}
