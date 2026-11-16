package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"fleet-dispatch-go/internal/repository"
	"fleet-dispatch-go/internal/service"
)

func newTestRouter(telemetryCh chan<- TelemetryEvent) http.Handler {
	repo := repository.NewMemoryRepository()
	svc := service.NewDispatchService(repo)
	return NewHandler(svc, nil, telemetryCh).Routes()
}

func TestHandler_DispatchFlow(t *testing.T) {
	telemetryCh := make(chan TelemetryEvent, 8)
	router := newTestRouter(telemetryCh)

	// 0. Health & List awal
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", rec.Code)
	}

	// 1. Pesan order sebelum ada driver -> harus 409 Conflict
	orderBody := `{"penumpang":"Andi","pickup_lat":-6.244,"pickup_lng":106.825,"dropoff_lat":-6.225,"dropoff_lng":106.830}`
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("POST", "/orders", bytes.NewBufferString(orderBody)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status tanpa driver = %d, want 409", rec.Code)
	}

	// 2. Daftarkan 1 driver -> 201 Created, duplikat nomor lambung -> 409 Conflict
	drvBody := `{"nama":"Pak Budi","nomor_lambung":"BBD-101","lat":-6.244,"lng":106.825}`
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("POST", "/drivers", bytes.NewBufferString(drvBody)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("register driver status = %d, want 201", rec.Code)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("POST", "/drivers", bytes.NewBufferString(drvBody)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate fleet number status = %d, want 409", rec.Code)
	}

	// 3. Update lokasi driver & pastikan event masuk telemetryCh
	locBody := `{"lat":-6.245,"lng":106.826,"status":"available"}`
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("PATCH", "/drivers/1/location", bytes.NewBufferString(locBody)))
	if rec.Code != http.StatusOK {
		t.Fatalf("update location status = %d, want 200", rec.Code)
	}
	select {
	case ev := <-telemetryCh:
		if ev.DriverID != 1 {
			t.Errorf("telemetry driver_id = %d, want 1", ev.DriverID)
		}
	default:
		t.Errorf("telemetry event tidak dikirim ke channel")
	}

	// 4. List drivers
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/drivers", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list drivers status = %d, want 200", rec.Code)
	}

	// 5. Pesan order lagi -> sekarang sukses 201 Created
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("POST", "/orders", bytes.NewBufferString(orderBody)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create order status = %d, want 201", rec.Code)
	}

	// 6. Loncat status langsung ke completed -> harus ditolak 409 Conflict
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("PATCH", "/orders/1/status", bytes.NewBufferString(`{"status":"completed"}`)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("loncat status = %d, want 409", rec.Code)
	}

	// 7. Jalankan alur sah: picked_up -> completed
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("PATCH", "/orders/1/status", bytes.NewBufferString(`{"status":"picked_up"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("picked_up status = %d, want 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("PATCH", "/orders/1/status", bytes.NewBufferString(`{"status":"completed"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("completed status = %d, want 200", rec.Code)
	}

	// 8. Cek detail order via GET /orders/1
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/orders/1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get order status = %d, want 200", rec.Code)
	}
	var resp struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || !resp.Success {
		t.Errorf("respons JSON tidak sesuai: %s", rec.Body.String())
	}
}

func TestHandler_ValidationAndErrors(t *testing.T) {
	router := newTestRouter(nil)

	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"missing lat/lng on register", "POST", "/drivers", `{"nama":"Budi","nomor_lambung":"BBD-101"}`, http.StatusBadRequest},
		{"trailing JSON object", "POST", "/drivers", `{"nama":"Budi","nomor_lambung":"BBD-101","lat":-6.2,"lng":106.8}{"extra":1}`, http.StatusBadRequest},
		{"unknown field", "POST", "/drivers", `{"nama":"Budi","nomor_lambung":"BBD-101","lat":-6.2,"lng":106.8,"hack":true}`, http.StatusBadRequest},
		{"invalid order id", "GET", "/orders/abc", "", http.StatusBadRequest},
		{"order not found", "GET", "/orders/999", "", http.StatusNotFound},
		{"driver not found on location", "PATCH", "/drivers/999/location", `{"lat":-6.2,"lng":106.8,"status":"available"}`, http.StatusNotFound},
		{"invalid status string on order", "PATCH", "/orders/1/status", `{"status":"bogus"}`, http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf *bytes.Buffer
			if tc.body != "" {
				buf = bytes.NewBufferString(tc.body)
			} else {
				buf = &bytes.Buffer{}
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, buf))
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d (body=%s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}
