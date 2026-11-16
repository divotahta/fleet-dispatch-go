package repository

import (
	"context"
	"fleet-dispatch-go/internal/domain"
)

type FleetRepository interface {
	CreateDriver(ctx context.Context, d *domain.Driver) error
	UpdateDriverLocation(ctx context.Context, id int64, lat, lng float64, status domain.DriverStatus) (*domain.Driver, error)
	ListDrivers(ctx context.Context) ([]domain.Driver, error)
	DispatchNearestDriver(ctx context.Context, order *domain.Order) (*domain.Driver, error)
	UpdateOrderStatus(ctx context.Context, orderID int64, next domain.OrderStatus) (*domain.Order, error)
	GetOrder(ctx context.Context, orderID int64) (*domain.Order, error)
}