package domain

import (
	"errors"
	"math"
	"testing"
)

func TestOrder_TransitionTo(t *testing.T) {
	cases := []struct {
		nama    string
		awal    OrderStatus
		tujuan  OrderStatus
		wantErr error
	}{
		{"assigned ke picked_up boleh", OrderAssigned, OrderPickedUp, nil},
		{"picked_up ke completed boleh", OrderPickedUp, OrderCompleted, nil},
		{"assigned ke cancelled boleh", OrderAssigned, OrderCancelled, nil},
		{"completed ke cancelled ditolak", OrderCompleted, OrderCancelled, ErrInvalidTransition},
		{"cancelled ke picked_up ditolak", OrderCancelled, OrderPickedUp, ErrInvalidTransition},
		{"assigned loncat ke completed ditolak", OrderAssigned, OrderCompleted, ErrInvalidTransition},
	}

	for _, tc := range cases {
		t.Run(tc.nama, func(t *testing.T) {
			order := &Order{Status: tc.awal}
			err := order.TransitionTo(tc.tujuan)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("TransitionTo() error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil && order.Status != tc.tujuan {
				t.Errorf("Status = %q, want %q", order.Status, tc.tujuan)
			}
		})
	}
}

func TestHitungTarif(t *testing.T) {
	cases := []struct {
		nama string
		km   float64
		want int
	}{
		{"nol km", 0, 8000},
		{"negatif jadi dasar", -3, 8000},
		{"NaN jadi dasar", math.NaN(), 8000},
		{"Inf jadi dasar", math.Inf(1), 8000},
		{"lima km", 5, 33000},
		{"pecahan 2.5 km", 2.5, 20500},
	}
	for _, tc := range cases {
		t.Run(tc.nama, func(t *testing.T) {
			if got := HitungTarif(tc.km); got != tc.want {
				t.Errorf("HitungTarif(%v) = %d, want %d", tc.km, got, tc.want)
			}
		})
	}
}

func TestHitungJarakKM(t *testing.T) {
	if got := HitungJarakKM(-6.244, 106.825, -6.244, 106.825); got != 0 {
		t.Errorf("jarak titik sama = %v, want 0", got)
	}
	dist := HitungJarakKM(-6.244, 106.825, -6.225, 106.830)
	if dist <= 1.5 || dist >= 3.0 {
		t.Errorf("jarak Mampang-Kuningan di luar rentang wajar: %v km", dist)
	}
}

func TestValidateCoordinatesAndStatuses(t *testing.T) {
	if err := ValidateCoordinates(-6.244, 106.825); err != nil {
		t.Errorf("koordinat valid dianggap error: %v", err)
	}
	for _, bad := range [][2]float64{{91, 0}, {-91, 0}, {0, 181}, {0, -181}, {math.NaN(), 0}, {0, math.Inf(1)}} {
		if err := ValidateCoordinates(bad[0], bad[1]); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("ValidateCoordinates(%v, %v) error = %v, want ErrInvalidInput", bad[0], bad[1], err)
		}
	}
	if !IsValidDriverStatus(DriverAvailable) || IsValidDriverStatus(DriverStatus("rusak")) {
		t.Errorf("IsValidDriverStatus hasil tidak sesuai")
	}
	if !IsValidOrderStatus(OrderCompleted) || IsValidOrderStatus(OrderStatus("bogus")) {
		t.Errorf("IsValidOrderStatus hasil tidak sesuai")
	}
}
