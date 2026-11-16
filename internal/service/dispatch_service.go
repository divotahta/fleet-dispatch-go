package service

import (
	"context"
	"fmt"
	"strings"

	"fleet-dispatch-go/internal/domain"
	"fleet-dispatch-go/internal/repository"
)

type CreateOrderInput struct {
	Penumpang  string  `json:"penumpang"`
	PickupLat  float64 `json:"pickup_lat"`
	PickupLng  float64 `json:"pickup_lng"`
	DropoffLat float64 `json:"dropoff_lat"`
	DropoffLng float64 `json:"dropoff_lng"`
}

type CreateOrderResult struct {
	Order  *domain.Order  `json:"order"`
	Driver *domain.Driver `json:"driver"`
}

type DispatchService struct {
	repo repository.FleetRepository
}

func NewDispatchService(repo repository.FleetRepository) *DispatchService {
	return &DispatchService{repo: repo}
}

func (s *DispatchService) RegisterDriver(ctx context.Context, nama, nomorLambung string, lat, lng float64) (*domain.Driver, error) {
	nama = strings.TrimSpace(nama)
	nomorLambung = strings.ToUpper(strings.TrimSpace(nomorLambung))
	if nama == "" || nomorLambung == "" {
		return nil, fmt.Errorf("%w: nama dan nomor_lambung wajib diisi", domain.ErrInvalidInput)
	}
	if err := domain.ValidateCoordinates(lat, lng); err != nil {
		return nil, err
	}

	d := &domain.Driver{
		Nama:         nama,
		NomorLambung: nomorLambung,
		Status:       domain.DriverAvailable,
		Lat:          lat,
		Lng:          lng,
	}
	if err := s.repo.CreateDriver(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *DispatchService) UpdateLocation(ctx context.Context, id int64, lat, lng float64, status domain.DriverStatus) (*domain.Driver, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: id tidak valid", domain.ErrInvalidInput)
	}
	if err := domain.ValidateCoordinates(lat, lng); err != nil {
		return nil, err
	}
	if !domain.IsValidDriverStatus(status) {
		return nil, fmt.Errorf("%w: status harus available, on_trip, atau offline", domain.ErrInvalidInput)
	}
	return s.repo.UpdateDriverLocation(ctx, id, lat, lng, status)
}

func (s *DispatchService) ListDrivers(ctx context.Context) ([]domain.Driver, error) {
	return s.repo.ListDrivers(ctx)
}

func (s *DispatchService) CreateOrder(ctx context.Context, input CreateOrderInput) (*CreateOrderResult, error) {
	penumpang := strings.TrimSpace(input.Penumpang)
	if penumpang == "" {
		return nil, fmt.Errorf("%w: nama penumpang wajib diisi", domain.ErrInvalidInput)
	}
	if err := domain.ValidateCoordinates(input.PickupLat, input.PickupLng); err != nil {
		return nil, fmt.Errorf("koordinat pickup: %w", err)
	}
	if err := domain.ValidateCoordinates(input.DropoffLat, input.DropoffLng); err != nil {
		return nil, fmt.Errorf("koordinat dropoff: %w", err)
	}

	jarak := domain.HitungJarakKM(input.PickupLat, input.PickupLng, input.DropoffLat, input.DropoffLng)
	order := &domain.Order{
		Penumpang:  penumpang,
		PickupLat:  input.PickupLat,
		PickupLng:  input.PickupLng,
		DropoffLat: input.DropoffLat,
		DropoffLng: input.DropoffLng,
		JarakKM:    jarak,
		Tarif:      domain.HitungTarif(jarak),
	}

	drv, err := s.repo.DispatchNearestDriver(ctx, order)
	if err != nil {
		return nil, err
	}

	return &CreateOrderResult{Order: order, Driver: drv}, nil
}

func (s *DispatchService) UpdateOrderStatus(ctx context.Context, orderID int64, next domain.OrderStatus) (*domain.Order, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("%w: order_id tidak valid", domain.ErrInvalidInput)
	}
	normalized := domain.OrderStatus(strings.TrimSpace(string(next)))
	if !domain.IsValidOrderStatus(normalized) {
		return nil, fmt.Errorf("%w: status order harus picked_up, completed, atau cancelled", domain.ErrInvalidInput)
	}
	return s.repo.UpdateOrderStatus(ctx, orderID, normalized)
}

func (s *DispatchService) GetOrder(ctx context.Context, orderID int64) (*domain.Order, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("%w: order_id tidak valid", domain.ErrInvalidInput)
	}
	return s.repo.GetOrder(ctx, orderID)
}
