package domain

import (
	"errors"
	"fmt"
	"math"
	"time"
)

type DriverStatus string

const (
	DriverAvailable DriverStatus = "available"
	DriverOnTrip    DriverStatus = "on_trip"
	DriverOffline   DriverStatus = "offline"
)

var (
	ErrDriverNotFound       = errors.New("driver tidak ditemukan")
	ErrNoDriverAvailable    = errors.New("tidak ada driver yang tersedia")
	ErrDuplicateFleetNumber = errors.New("nomor lambung sudah terdaftar")
	ErrOrderNotFound        = errors.New("order tidak ditemukan")
	ErrInvalidInput         = errors.New("input tidak valid")
	ErrInvalidTransition    = errors.New("perubahan status tidak diizinkan")
)

func IsValidDriverStatus(s DriverStatus) bool {
	switch s {
	case DriverAvailable, DriverOnTrip, DriverOffline:
		return true
	default:
		return false
	}
}

// ValidateCoordinates memastikan latitude [-90, 90] dan longitude [-180, 180] valid serta bukan NaN/Inf.
func ValidateCoordinates(lat, lng float64) error {
	if math.IsNaN(lat) || math.IsInf(lat, 0) || lat < -90 || lat > 90 {
		return fmt.Errorf("%w: latitude harus antara -90 dan 90", ErrInvalidInput)
	}
	if math.IsNaN(lng) || math.IsInf(lng, 0) || lng < -180 || lng > 180 {
		return fmt.Errorf("%w: longitude harus antara -180 dan 180", ErrInvalidInput)
	}
	return nil
}

type Driver struct {
	ID           int64        `json:"id"`
	Nama         string       `json:"nama"`
	NomorLambung string       `json:"nomor_lambung"`
	Status       DriverStatus `json:"status"`
	Lat          float64      `json:"lat"`
	Lng          float64      `json:"lng"`
	UpdatedAt    time.Time    `json:"updated_at"`
}
