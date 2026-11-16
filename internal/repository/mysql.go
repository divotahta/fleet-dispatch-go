package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"fleet-dispatch-go/internal/domain"
	"github.com/go-sql-driver/mysql"
)

// MySQLRepository mengimplementasikan FleetRepository menggunakan transaksi MySQL dan row locking.
type MySQLRepository struct {
	db *sql.DB
}

// NewMySQLRepository membuat instance MySQLRepository baru.
func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

func (r *MySQLRepository) CreateDriver(ctx context.Context, d *domain.Driver) error {
	const query = `
		INSERT INTO drivers (nama, nomor_lambung, status, lat, lng)
		VALUES (?, ?, ?, ?, ?)
	`

	res, err := r.db.ExecContext(ctx, query, d.Nama, d.NomorLambung, d.Status, d.Lat, d.Lng)
	if err != nil {
		if isMySQLDuplicateEntry(err) {
			return domain.ErrDuplicateFleetNumber
		}
		return fmt.Errorf("insert driver: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("fetch driver id: %w", err)
	}

	created, err := r.findDriverByID(ctx, r.db, id, false)
	if err != nil {
		return err
	}
	*d = *created
	return nil
}

func (r *MySQLRepository) UpdateDriverLocation(ctx context.Context, id int64, lat, lng float64, status domain.DriverStatus) (*domain.Driver, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	current, err := r.findDriverByID(ctx, tx, id, true)
	if err != nil {
		return nil, err
	}

	if current.Status == domain.DriverOnTrip && status != domain.DriverOnTrip {
		return nil, fmt.Errorf("%w: driver sedang on_trip", domain.ErrInvalidTransition)
	}
	if current.Status != domain.DriverOnTrip && status == domain.DriverOnTrip {
		return nil, fmt.Errorf("%w: status on_trip hanya diatur melalui order", domain.ErrInvalidTransition)
	}

	const updateQuery = `
		UPDATE drivers
		SET lat = ?, lng = ?, status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	if _, err := tx.ExecContext(ctx, updateQuery, lat, lng, status, id); err != nil {
		return nil, fmt.Errorf("update driver location: %w", err)
	}

	updated, err := r.findDriverByID(ctx, tx, id, false)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return updated, nil
}

func (r *MySQLRepository) ListDrivers(ctx context.Context) ([]domain.Driver, error) {
	const query = `
		SELECT id, nama, nomor_lambung, status, lat, lng, updated_at
		FROM drivers
		ORDER BY id ASC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list drivers: %w", err)
	}
	defer rows.Close()

	drivers := make([]domain.Driver, 0)
	for rows.Next() {
		var d domain.Driver
		if err := rows.Scan(&d.ID, &d.Nama, &d.NomorLambung, &d.Status, &d.Lat, &d.Lng, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan driver: %w", err)
		}
		drivers = append(drivers, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate drivers: %w", err)
	}
	return drivers, nil
}

func (r *MySQLRepository) DispatchNearestDriver(ctx context.Context, order *domain.Order) (*domain.Driver, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin dispatch tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Kunci baris driver yang tersedia selama transaksi agar tidak terjadi double booking.
	const selectAvailable = `
		SELECT id, nama, nomor_lambung, status, lat, lng, updated_at
		FROM drivers
		WHERE status = ?
		ORDER BY id ASC
		FOR UPDATE
	`

	rows, err := tx.QueryContext(ctx, selectAvailable, domain.DriverAvailable)
	if err != nil {
		return nil, fmt.Errorf("query available drivers: %w", err)
	}

	var nearest *domain.Driver
	bestDist := math.MaxFloat64

	for rows.Next() {
		var d domain.Driver
		if err := rows.Scan(&d.ID, &d.Nama, &d.NomorLambung, &d.Status, &d.Lat, &d.Lng, &d.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan available driver: %w", err)
		}
		dist := domain.HitungJarakKM(order.PickupLat, order.PickupLng, d.Lat, d.Lng)
		if dist < bestDist || (dist == bestDist && (nearest == nil || d.ID < nearest.ID)) {
			bestDist = dist
			candidate := d
			nearest = &candidate
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate available drivers: %w", err)
	}
	_ = rows.Close()

	if nearest == nil {
		return nil, domain.ErrNoDriverAvailable
	}

	const lockDriver = `
		UPDATE drivers
		SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	if _, err := tx.ExecContext(ctx, lockDriver, domain.DriverOnTrip, nearest.ID); err != nil {
		return nil, fmt.Errorf("lock driver: %w", err)
	}

	const insertOrder = `
		INSERT INTO orders (driver_id, penumpang, pickup_lat, pickup_lng, dropoff_lat, dropoff_lng, jarak_km, tarif, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	res, err := tx.ExecContext(ctx, insertOrder,
		nearest.ID,
		order.Penumpang,
		order.PickupLat,
		order.PickupLng,
		order.DropoffLat,
		order.DropoffLng,
		order.JarakKM,
		order.Tarif,
		domain.OrderAssigned,
	)
	if err != nil {
		return nil, fmt.Errorf("insert order: %w", err)
	}

	orderID, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("fetch order id: %w", err)
	}

	createdOrder, err := r.findOrderByID(ctx, tx, orderID, false)
	if err != nil {
		return nil, err
	}
	updatedDriver, err := r.findDriverByID(ctx, tx, nearest.ID, false)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit dispatch tx: %w", err)
	}

	*order = *createdOrder
	return updatedDriver, nil
}

func (r *MySQLRepository) UpdateOrderStatus(ctx context.Context, orderID int64, next domain.OrderStatus) (*domain.Order, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin order tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	ord, err := r.findOrderByID(ctx, tx, orderID, true)
	if err != nil {
		return nil, err
	}

	if err := ord.TransitionTo(next); err != nil {
		return nil, err
	}

	const updateOrder = `
		UPDATE orders
		SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	if _, err := tx.ExecContext(ctx, updateOrder, next, orderID); err != nil {
		return nil, fmt.Errorf("update order status: %w", err)
	}

	if next == domain.OrderCompleted || next == domain.OrderCancelled {
		const releaseDriver = `
			UPDATE drivers
			SET status = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`
		if _, err := tx.ExecContext(ctx, releaseDriver, domain.DriverAvailable, ord.DriverID); err != nil {
			return nil, fmt.Errorf("release driver: %w", err)
		}
	}

	updated, err := r.findOrderByID(ctx, tx, orderID, false)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit order tx: %w", err)
	}
	return updated, nil
}

func (r *MySQLRepository) GetOrder(ctx context.Context, orderID int64) (*domain.Order, error) {
	return r.findOrderByID(ctx, r.db, orderID, false)
}

type queryRunner interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (r *MySQLRepository) findDriverByID(ctx context.Context, q queryRunner, id int64, forUpdate bool) (*domain.Driver, error) {
	query := `
		SELECT id, nama, nomor_lambung, status, lat, lng, updated_at
		FROM drivers
		WHERE id = ?
	`
	if forUpdate {
		query += ` FOR UPDATE`
	}

	var d domain.Driver
	err := q.QueryRowContext(ctx, query, id).Scan(
		&d.ID, &d.Nama, &d.NomorLambung, &d.Status, &d.Lat, &d.Lng, &d.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrDriverNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find driver: %w", err)
	}
	return &d, nil
}

func (r *MySQLRepository) findOrderByID(ctx context.Context, q queryRunner, id int64, forUpdate bool) (*domain.Order, error) {
	query := `
		SELECT id, driver_id, penumpang, pickup_lat, pickup_lng, dropoff_lat, dropoff_lng, jarak_km, tarif, status, created_at, updated_at
		FROM orders
		WHERE id = ?
	`
	if forUpdate {
		query += ` FOR UPDATE`
	}

	var ord domain.Order
	err := q.QueryRowContext(ctx, query, id).Scan(
		&ord.ID, &ord.DriverID, &ord.Penumpang,
		&ord.PickupLat, &ord.PickupLng, &ord.DropoffLat, &ord.DropoffLng,
		&ord.JarakKM, &ord.Tarif, &ord.Status, &ord.CreatedAt, &ord.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrOrderNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find order: %w", err)
	}
	return &ord, nil
}

func isMySQLDuplicateEntry(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
