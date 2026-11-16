# Fleet Dispatch Service (`fleet-dispatch-go`)

Layanan backend Go untuk simulasi dispatch armada taksi: mencari driver `available` terdekat dari titik jemput penumpang, mencegah *double booking* saat banyak request masuk bersamaan, dan menjaga urutan status perjalanan lewat *state machine*.

Proyek ini dibangun dengan standard library Go (`net/http`, `sync`, `log/slog`, `testing`) tanpa framework web eksternal.

## Fitur Utama

1. **Manajemen Driver & Lokasi**
   - Registrasi driver baru (`POST /drivers`).
   - Pembaruan koordinat GPS dan status (`available`, `on_trip`, `offline`) melalui `PATCH /drivers/{id}/location` yang mengirim event secara non-blocking ke *buffered channel* telemetry worker.
2. **Auto-Dispatch Terdekat & Anti Double-Booking**
   - `POST /orders` menghitung jarak titik jemput ke seluruh driver `available` dengan rumus Haversine dan memilih yang terdekat.
   - Pencarian dan penguncian status driver (`available` ke `on_trip`) dibungkus di dalam `sync.Mutex` agar request paralel tidak bisa mengambil driver yang sama.
3. **State Machine Status Perjalanan**
   - `PATCH /orders/{id}/status` hanya mengizinkan transisi:
     - `assigned` -> `picked_up` -> `completed`
     - `assigned` -> `cancelled`
   - Saat order berstatus `completed` atau `cancelled`, driver otomatis kembali menjadi `available`.

## Struktur Proyek

```text
fleet-dispatch-go/
├─ cmd/
│  └─ api/
│     └─ main.go                      # Wiring dependency, telemetry worker, graceful shutdown
├─ internal/
│  ├─ domain/
│  │  ├─ driver.go                    # Struct Driver, DriverStatus, sentinel errors
│  │  ├─ order.go                     # Struct Order, state machine, rumus Haversine & tarif
│  │  └─ order_test.go                # Unit test state machine & tarif
│  ├─ repository/
│  │  ├─ interfaces.go                # Interface FleetRepository
│  │  └─ memory.go                    # Implementasi thread-safe in-memory (sync.Mutex)
│  ├─ service/
│  │  ├─ dispatch_service.go          # Logika bisnis & validasi input
│  │  └─ dispatch_service_test.go     # Unit test + simulasi 20 goroutine berebut 1 driver
│  └─ handler/
│     ├─ http_handler.go              # Router net/http, JSON response, logging middleware
│     └─ http_handler_test.go         # Pengujian alur endpoint dengan httptest
├─ Dockerfile
├─ fleet-dispatch-go.postman_collection.json
├─ go.mod
└─ README.md
```

## Cara Menjalankan

### Lokal (Go 1.22+)

```powershell
# Jalankan seluruh unit test dan concurrency test
go test -v -cover ./...

# Jalankan HTTP server di http://localhost:8080
go run ./cmd/api
```

### Docker

```bash
docker build -t fleet-dispatch-go .
docker run -p 8080:8080 fleet-dispatch-go
```

## Daftar Endpoint

| Method | Endpoint | Fungsi | Status Sukses | Error Umum |
|---|---|---|---|---|
| `GET` | `/health` | Cek status server | `200 OK` | - |
| `POST` | `/drivers` | Daftarkan driver baru | `201 Created` | `400` |
| `GET` | `/drivers` | Lihat daftar seluruh driver | `200 OK` | - |
| `PATCH` | `/drivers/{id}/location` | Perbarui koordinat & status driver | `200 OK` | `400`, `404` |
| `POST` | `/orders` | Pesan taksi & auto-dispatch terdekat | `201 Created` | `400`, `409` |
| `GET` | `/orders/{id}` | Lihat detail order | `200 OK` | `400`, `404` |
| `PATCH` | `/orders/{id}/status` | Ubah status order (`picked_up`, `completed`, `cancelled`) | `200 OK` | `400`, `404`, `409` |

Untuk pengujian lewat Postman, langsung import file `fleet-dispatch-go.postman_collection.json`.

## Contoh Pengujian dengan `curl`

```bash
# 1. Daftarkan driver
curl -i -X POST http://localhost:8080/drivers \
  -H "Content-Type: application/json" \
  -d '{"nama":"Pak Budi","nomor_lambung":"BBD-101","lat":-6.244,"lng":106.825}'

# 2. Buat order (auto-dispatch driver terdekat)
curl -i -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{"penumpang":"Andi","pickup_lat":-6.244,"pickup_lng":106.825,"dropoff_lat":-6.225,"dropoff_lng":106.830}'

# 3. Ubah status order: picked_up -> completed
curl -i -X PATCH http://localhost:8080/orders/1/status \
  -H "Content-Type: application/json" \
  -d '{"status":"picked_up"}'

curl -i -X PATCH http://localhost:8080/orders/1/status \
  -H "Content-Type: application/json" \
  -d '{"status":"completed"}'
```
