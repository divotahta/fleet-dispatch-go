package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"fleet-dispatch-go/internal/domain"
	"fleet-dispatch-go/internal/repository"
)

func TestDispatchService_RegisterAndValidation(t *testing.T) {
	ctx := context.Background()
	svc := NewDispatchService(repository.NewMemoryRepository())

	d, err := svc.RegisterDriver(ctx, "  Pak Budi ", " bbd-101 ", -6.244, 106.825)
	if err != nil {
		t.Fatalf("RegisterDriver() error = %v", err)
	}
	if d.Nama != "Pak Budi" || d.NomorLambung != "BBD-101" {
		t.Errorf("normalisasi gagal: %+v", d)
	}

	if _, err := svc.RegisterDriver(ctx, "Driver Dua", "bbd-101", -6.244, 106.825); !errors.Is(err, domain.ErrDuplicateFleetNumber) {
		t.Errorf("duplikat nomor lambung error = %v, want ErrDuplicateFleetNumber", err)
	}
	if _, err := svc.RegisterDriver(ctx, "", "BBD-102", -6.244, 106.825); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("nama kosong error = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.RegisterDriver(ctx, "Andi", "BBD-103", 999, 106.825); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("lat out of range error = %v, want ErrInvalidInput", err)
	}
}

func TestDispatchService_PicksNearestAvailableDriver(t *testing.T) {
	ctx := context.Background()
	svc := NewDispatchService(repository.NewMemoryRepository())

	// Driver 1 sangat dekat tapi offline -> harus diabaikan
	off, _ := svc.RegisterDriver(ctx, "Pak Offline", "BBD-000", -6.2440, 106.8250)
	_, _ = svc.UpdateLocation(ctx, off.ID, -6.2440, 106.8250, domain.DriverOffline)

	// Driver 2 jauh (Bogor), Driver 3 dekat (Mampang)
	_, _ = svc.RegisterDriver(ctx, "Pak Jauh", "BBD-001", -6.5971, 106.8060)
	dekat, _ := svc.RegisterDriver(ctx, "Pak Dekat", "BBD-002", -6.2450, 106.8260)

	res, err := svc.CreateOrder(ctx, CreateOrderInput{
		Penumpang:  "Andi",
		PickupLat:  -6.2440,
		PickupLng:  106.8250,
		DropoffLat: -6.2250,
		DropoffLng: 106.8300,
	})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	if res.Driver.ID != dekat.ID {
		t.Errorf("terpilih driver ID %d, maunya ID %d (Pak Dekat)", res.Driver.ID, dekat.ID)
	}
}

func TestDispatchService_ConcurrentAntiDoubleBooking(t *testing.T) {
	ctx := context.Background()
	svc := NewDispatchService(repository.NewMemoryRepository())

	_, _ = svc.RegisterDriver(ctx, "Pak Budi", "BBD-101", -6.2440, 106.8250)

	const totalPenumpang = 20
	var suksesCount atomic.Int64
	var gagalCount atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < totalPenumpang; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.CreateOrder(ctx, CreateOrderInput{
				Penumpang:  "Penumpang",
				PickupLat:  -6.2440,
				PickupLng:  106.8250,
				DropoffLat: -6.2250,
				DropoffLng: 106.8300,
			})
			if err == nil {
				suksesCount.Add(1)
				return
			}
			if errors.Is(err, domain.ErrNoDriverAvailable) {
				gagalCount.Add(1)
			} else {
				t.Errorf("error tidak terduga: %v", err)
			}
		}()
	}

	wg.Wait()

	if suksesCount.Load() != 1 {
		t.Errorf("sukses = %d, harus tepat 1!", suksesCount.Load())
	}
	if gagalCount.Load() != totalPenumpang-1 {
		t.Errorf("gagal = %d, harus %d", gagalCount.Load(), totalPenumpang-1)
	}
}

func TestDispatchService_StatusInvariantsAndRelease(t *testing.T) {
	ctx := context.Background()
	svc := NewDispatchService(repository.NewMemoryRepository())

	drv, _ := svc.RegisterDriver(ctx, "Pak Budi", "BBD-101", -6.2440, 106.8250)

	// Tidak boleh set on_trip manual sebelum ada order
	if _, err := svc.UpdateLocation(ctx, drv.ID, -6.244, 106.825, domain.DriverOnTrip); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("set on_trip manual error = %v, want ErrInvalidTransition", err)
	}

	order1, err := svc.CreateOrder(ctx, CreateOrderInput{
		Penumpang: "Penumpang 1", PickupLat: -6.2440, PickupLng: 106.8250,
		DropoffLat: -6.2250, DropoffLng: 106.8300,
	})
	if err != nil {
		t.Fatalf("order 1 gagal: %v", err)
	}

	// Saat on_trip, tidak boleh diubah manual jadi available/offline lewat UpdateLocation
	if _, err := svc.UpdateLocation(ctx, drv.ID, -6.240, 106.826, domain.DriverAvailable); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("ubah paksa available saat on_trip error = %v, want ErrInvalidTransition", err)
	}

	// Invalid order status harus ErrInvalidInput (400), bukan ErrInvalidTransition (409)
	if _, err := svc.UpdateOrderStatus(ctx, order1.Order.ID, domain.OrderStatus("bogus")); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("status bogus error = %v, want ErrInvalidInput", err)
	}

	// Cancel order membebaskan driver kembali available
	if _, err := svc.UpdateOrderStatus(ctx, order1.Order.ID, domain.OrderCancelled); err != nil {
		t.Fatalf("cancel order 1 gagal: %v", err)
	}

	order2, err := svc.CreateOrder(ctx, CreateOrderInput{
		Penumpang: "Penumpang 2", PickupLat: -6.2440, PickupLng: 106.8250,
		DropoffLat: -6.2250, DropoffLng: 106.8300,
	})
	if err != nil {
		t.Fatalf("order 2 setelah cancel gagal: %v", err)
	}
	if _, err := svc.UpdateOrderStatus(ctx, order2.Order.ID, domain.OrderPickedUp); err != nil {
		t.Fatalf("picked_up gagal: %v", err)
	}
	if _, err := svc.UpdateOrderStatus(ctx, order2.Order.ID, domain.OrderCompleted); err != nil {
		t.Fatalf("completed gagal: %v", err)
	}

	got, err := svc.GetOrder(ctx, order2.Order.ID)
	if err != nil || got.Status != domain.OrderCompleted {
		t.Errorf("GetOrder() = %+v, err = %v", got, err)
	}
}
