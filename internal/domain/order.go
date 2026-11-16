package domain

import (
	"math"
	"time"
)

type OrderStatus string

const (
	OrderAssigned  OrderStatus = "assigned"
	OrderPickedUp  OrderStatus = "picked_up"
	OrderCompleted OrderStatus = "completed"
	OrderCancelled OrderStatus = "cancelled"
)

// IsValidOrderStatus memeriksa apakah status order termasuk nilai yang dikenali sistem.
func IsValidOrderStatus(s OrderStatus) bool {
	switch s {
	case OrderAssigned, OrderPickedUp, OrderCompleted, OrderCancelled:
		return true
	default:
		return false
	}
}

// Peta transisi status yang diizinkan (State Machine)
var allowedTransitions = map[OrderStatus][]OrderStatus{
	OrderAssigned: {OrderPickedUp, OrderCancelled},
	OrderPickedUp: {OrderCompleted},
}

type Order struct {
	ID         int64       `json:"id"`
	DriverID   int64       `json:"driver_id"`
	Penumpang  string      `json:"penumpang"`
	PickupLat  float64     `json:"pickup_lat"`
	PickupLng  float64     `json:"pickup_lng"`
	DropoffLat float64     `json:"dropoff_lat"`
	DropoffLng float64     `json:"dropoff_lng"`
	JarakKM    float64     `json:"jarak_km"`
	Tarif      int         `json:"tarif"`
	Status     OrderStatus `json:"status"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

// TransitionTo memvalidasi apakah perubahan status order diperbolehkan.
func (o *Order) TransitionTo(next OrderStatus) error {
	for _, allowed := range allowedTransitions[o.Status] {
		if allowed == next {
			o.Status = next
			o.UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return ErrInvalidTransition
}

// HitungJarakKM menghitung jarak garis lurus antar 2 koordinat GPS (rumus Haversine).
func HitungJarakKM(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKM = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLng := (lng2 - lng1) * math.Pi / 180.0

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180.0)*math.Cos(lat2*math.Pi/180.0)*
			math.Sin(dLng/2)*math.Sin(dLng/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return math.Round(earthRadiusKM*c*100) / 100
}

// HitungTarif menghitung tarif dasar (buka pintu Rp8.000 + Rp5.000/km).
func HitungTarif(jarakKM float64) int {
	const tarifDasar = 8000
	const tarifPerKM = 5000
	if math.IsNaN(jarakKM) || math.IsInf(jarakKM, 0) || jarakKM < 0 {
		jarakKM = 0
	}
	return tarifDasar + int(math.Round(jarakKM*tarifPerKM))
}
