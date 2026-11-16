package repository

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"fleet-dispatch-go/internal/domain"
)

type MemoryRepository struct {
	mu           sync.Mutex
	drivers      map[int64]*domain.Driver
	orders       map[int64]*domain.Order
	nextDriverID int64
	nextOrderID  int64
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		drivers:      make(map[int64]*domain.Driver),
		orders:       make(map[int64]*domain.Order),
		nextDriverID: 1,
		nextOrderID:  1,
	}
}

func (r *MemoryRepository) CreateDriver(_ context.Context, d *domain.Driver) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.drivers {
		if strings.EqualFold(existing.NomorLambung, d.NomorLambung) {
			return domain.ErrDuplicateFleetNumber
		}
	}

	d.ID = r.nextDriverID
	d.UpdatedAt = time.Now().UTC()
	r.nextDriverID++

	copyDriver := *d
	r.drivers[d.ID] = &copyDriver
	return nil
}

func (r *MemoryRepository) UpdateDriverLocation(_ context.Context, id int64, lat, lng float64, status domain.DriverStatus) (*domain.Driver, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	d, ok := r.drivers[id]
	if !ok {
		return nil, domain.ErrDriverNotFound
	}

	// Jaga invarian: driver yang sedang on_trip tidak boleh diubah manual menjadi available/offline,
	// dan status on_trip hanya boleh diatur oleh proses dispatch order.
	if d.Status == domain.DriverOnTrip && status != domain.DriverOnTrip {
		return nil, fmt.Errorf("%w: driver sedang on_trip", domain.ErrInvalidTransition)
	}
	if d.Status != domain.DriverOnTrip && status == domain.DriverOnTrip {
		return nil, fmt.Errorf("%w: status on_trip hanya diatur melalui order", domain.ErrInvalidTransition)
	}

	d.Lat = lat
	d.Lng = lng
	d.Status = status
	d.UpdatedAt = time.Now().UTC()

	copyDriver := *d
	return &copyDriver, nil
}

func (r *MemoryRepository) ListDrivers(_ context.Context) ([]domain.Driver, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	hasil := make([]domain.Driver, 0, len(r.drivers))
	for _, d := range r.drivers {
		hasil = append(hasil, *d)
	}
	return hasil, nil
}

func (r *MemoryRepository) DispatchNearestDriver(_ context.Context, order *domain.Order) (*domain.Driver, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var nearest *domain.Driver
	bestDist := math.MaxFloat64

	for _, d := range r.drivers {
		if d.Status != domain.DriverAvailable {
			continue
		}
		dist := domain.HitungJarakKM(order.PickupLat, order.PickupLng, d.Lat, d.Lng)
		if dist < bestDist || (dist == bestDist && (nearest == nil || d.ID < nearest.ID)) {
			bestDist = dist
			nearest = d
		}
	}

	if nearest == nil {
		return nil, domain.ErrNoDriverAvailable
	}

	now := time.Now().UTC()
	nearest.Status = domain.DriverOnTrip
	nearest.UpdatedAt = now

	order.ID = r.nextOrderID
	order.DriverID = nearest.ID
	order.Status = domain.OrderAssigned
	order.CreatedAt = now
	order.UpdatedAt = now
	r.nextOrderID++

	copyOrder := *order
	r.orders[order.ID] = &copyOrder

	copyDriver := *nearest
	return &copyDriver, nil
}

func (r *MemoryRepository) UpdateOrderStatus(_ context.Context, orderID int64, next domain.OrderStatus) (*domain.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ord, ok := r.orders[orderID]
	if !ok {
		return nil, domain.ErrOrderNotFound
	}
	if err := ord.TransitionTo(next); err != nil {
		return nil, err
	}

	if next == domain.OrderCompleted || next == domain.OrderCancelled {
		if drv, exists := r.drivers[ord.DriverID]; exists {
			drv.Status = domain.DriverAvailable
			drv.UpdatedAt = time.Now().UTC()
		}
	}

	copyOrder := *ord
	return &copyOrder, nil
}

func (r *MemoryRepository) GetOrder(_ context.Context, orderID int64) (*domain.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ord, ok := r.orders[orderID]
	if !ok {
		return nil, domain.ErrOrderNotFound
	}
	copyOrder := *ord
	return &copyOrder, nil
}
