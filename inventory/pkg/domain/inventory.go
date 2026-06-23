package domain

import "time"

type StockItem struct {
	SKU       string    `json:"sku"`
	Quantity  int       `json:"quantity"`
	Reserved  int       `json:"reserved"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (i StockItem) Available() int {
	return i.Quantity - i.Reserved
}

type ReservationStatus string

const (
	ReservationActive    ReservationStatus = "active"
	ReservationCommitted ReservationStatus = "committed"
	ReservationReleased  ReservationStatus = "released"
)

type ReservationItem struct {
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
}

type Reservation struct {
	ID        string            `json:"id"`
	OrderID   string            `json:"order_id"`
	Items     []ReservationItem `json:"items"`
	Status    ReservationStatus `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

func NewReservation(id, orderID string, items []ReservationItem, now time.Time) *Reservation {
	copiedItems := make([]ReservationItem, len(items))
	copy(copiedItems, items)

	return &Reservation{
		ID:        id,
		OrderID:   orderID,
		Items:     copiedItems,
		Status:    ReservationActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (r Reservation) IsActive() bool {
	return r.Status == ReservationActive
}
