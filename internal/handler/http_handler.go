package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"fleet-dispatch-go/internal/domain"
	"fleet-dispatch-go/internal/service"
)

type TelemetryEvent struct {
	DriverID int64               `json:"driver_id"`
	Lat      float64             `json:"lat"`
	Lng      float64             `json:"lng"`
	Status   domain.DriverStatus `json:"status"`
}

type Handler struct {
	svc         *service.DispatchService
	logger      *slog.Logger
	telemetryCh chan<- TelemetryEvent
}

func NewHandler(svc *service.DispatchService, logger *slog.Logger, telemetryCh chan<- TelemetryEvent) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		svc:         svc,
		logger:      logger,
		telemetryCh: telemetryCh,
	}
}

type successResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorResponse struct {
	Success bool        `json:"success"`
	Error   errorDetail `json:"error"`
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.handleHealth)
	mux.HandleFunc("POST /drivers", h.handleRegisterDriver)
	mux.HandleFunc("GET /drivers", h.handleListDrivers)
	mux.HandleFunc("PATCH /drivers/{id}/location", h.handleUpdateLocation)
	mux.HandleFunc("POST /orders", h.handleCreateOrder)
	mux.HandleFunc("GET /orders/{id}", h.handleGetOrder)
	mux.HandleFunc("PATCH /orders/{id}/status", h.handleUpdateOrderStatus)
	return h.loggingMiddleware(mux)
}

func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, successResponse{Success: true, Data: map[string]string{"status": "ok"}})
}

func (h *Handler) handleRegisterDriver(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Nama         string   `json:"nama"`
		NomorLambung string   `json:"nomor_lambung"`
		Lat          *float64 `json:"lat"`
		Lng          *float64 `json:"lng"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if body.Lat == nil || body.Lng == nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "lat dan lng wajib diisi")
		return
	}

	drv, err := h.svc.RegisterDriver(r.Context(), body.Nama, body.NomorLambung, *body.Lat, *body.Lng)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, successResponse{Success: true, Message: "driver terdaftar", Data: drv})
}

func (h *Handler) handleListDrivers(w http.ResponseWriter, r *http.Request) {
	drivers, err := h.svc.ListDrivers(r.Context())
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Success: true, Data: drivers})
}

func (h *Handler) handleUpdateLocation(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	var body struct {
		Lat    *float64            `json:"lat"`
		Lng    *float64            `json:"lng"`
		Status domain.DriverStatus `json:"status"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if body.Lat == nil || body.Lng == nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "lat dan lng wajib diisi")
		return
	}

	drv, err := h.svc.UpdateLocation(r.Context(), id, *body.Lat, *body.Lng, body.Status)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	// Kirim event lokasi ke worker channel tanpa memblokir HTTP response
	if h.telemetryCh != nil {
		select {
		case h.telemetryCh <- TelemetryEvent{DriverID: drv.ID, Lat: drv.Lat, Lng: drv.Lng, Status: drv.Status}:
		default:
			h.logger.Warn("telemetry buffer penuh, event diabaikan", "driver_id", drv.ID)
		}
	}

	writeJSON(w, http.StatusOK, successResponse{Success: true, Message: "lokasi driver diperbarui", Data: drv})
}

func (h *Handler) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Penumpang  string   `json:"penumpang"`
		PickupLat  *float64 `json:"pickup_lat"`
		PickupLng  *float64 `json:"pickup_lng"`
		DropoffLat *float64 `json:"dropoff_lat"`
		DropoffLng *float64 `json:"dropoff_lng"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if body.PickupLat == nil || body.PickupLng == nil || body.DropoffLat == nil || body.DropoffLng == nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "koordinat pickup dan dropoff wajib diisi")
		return
	}

	res, err := h.svc.CreateOrder(r.Context(), service.CreateOrderInput{
		Penumpang:  body.Penumpang,
		PickupLat:  *body.PickupLat,
		PickupLng:  *body.PickupLng,
		DropoffLat: *body.DropoffLat,
		DropoffLng: *body.DropoffLng,
	})
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, successResponse{Success: true, Message: "driver berhasil ditugaskan", Data: res})
}

func (h *Handler) handleGetOrder(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	ord, err := h.svc.GetOrder(r.Context(), id)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Success: true, Data: ord})
}

func (h *Handler) handleUpdateOrderStatus(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	var body struct {
		Status domain.OrderStatus `json:"status"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	ord, err := h.svc.UpdateOrderStatus(r.Context(), id, body.Status)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Success: true, Message: "status order diperbarui", Data: ord})
}

func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, domain.ErrDriverNotFound), errors.Is(err, domain.ErrOrderNotFound):
		writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, domain.ErrNoDriverAvailable), errors.Is(err, domain.ErrInvalidTransition), errors.Is(err, domain.ErrDuplicateFleetNumber):
		writeError(w, http.StatusConflict, "CONFLICT", err.Error())
	default:
		h.logger.Error("internal error", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "terjadi kesalahan pada server")
	}
}

func (h *Handler) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		h.logger.Info("http request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration_ms", time.Since(start).Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

func (sr *statusRecorder) Unwrap() http.ResponseWriter {
	return sr.ResponseWriter
}

func parseID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("parameter id harus angka positif")
	}
	return id, nil
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errors.New("format JSON tidak valid")
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body hanya boleh berisi satu objek JSON")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, statusCode int, code, message string) {
	writeJSON(w, statusCode, errorResponse{
		Success: false,
		Error:   errorDetail{Code: code, Message: message},
	})
}
