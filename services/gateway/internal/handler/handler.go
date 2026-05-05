package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/parkir-pintar/gateway/internal/grpccall"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Handler is the REST-to-gRPC gateway.
type Handler struct {
	search      *grpc.ClientConn
	reservation *grpc.ClientConn
	billing     *grpc.ClientConn
	payment     *grpc.ClientConn
	presence    *grpc.ClientConn
}

func New(search, reservation, billing, payment, presence *grpc.ClientConn) *Handler {
	return &Handler{
		search:      search,
		reservation: reservation,
		billing:     billing,
		payment:     payment,
		presence:    presence,
	}
}

// Register mounts all REST routes on the given mux.
func (h *Handler) Register(mux *http.ServeMux) {
	// Search
	mux.HandleFunc("GET /v1/availability", h.getAvailability)
	mux.HandleFunc("GET /v1/availability/first", h.getFirstAvailable)

	// Reservation
	mux.HandleFunc("POST /v1/reservations", h.createReservation)
	mux.HandleFunc("GET /v1/reservations/", h.getReservation)
	mux.HandleFunc("DELETE /v1/reservations/", h.cancelReservation)
	mux.HandleFunc("POST /v1/spots/", h.holdSpot)

	// Check-in (via Presence)
	mux.HandleFunc("POST /v1/checkin", h.checkIn)

	// Billing
	mux.HandleFunc("POST /v1/checkout", h.checkout)

	// Presence
	mux.HandleFunc("POST /v1/checkout/gate", h.checkOut)
	mux.HandleFunc("POST /v1/presence/location", h.updateLocation)

	// Payment
	mux.HandleFunc("GET /v1/payments/", h.getPaymentStatus)
	mux.HandleFunc("POST /v1/payments/", h.handlePaymentPost)
}

// --- Search ---

func (h *Handler) getAvailability(w http.ResponseWriter, r *http.Request) {
	req := map[string]interface{}{
		"floor":        parseIntQuery(r, "floor", 0),
		"vehicle_type": r.URL.Query().Get("vehicle_type"),
	}
	var resp json.RawMessage
	if err := grpccall.Invoke(r.Context(), h.search, "/search.SearchService/GetAvailability", req, &resp); err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) getFirstAvailable(w http.ResponseWriter, r *http.Request) {
	req := map[string]interface{}{
		"vehicle_type": r.URL.Query().Get("vehicle_type"),
	}
	var resp json.RawMessage
	if err := grpccall.Invoke(r.Context(), h.search, "/search.SearchService/GetFirstAvailable", req, &resp); err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- Reservation ---

func (h *Handler) createReservation(w http.ResponseWriter, r *http.Request) {
	req, err := readBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Validate required fields
	if strField(req, "driver_id") == "" {
		writeError(w, http.StatusBadRequest, "driver_id is required")
		return
	}
	if strField(req, "mode") == "" {
		writeError(w, http.StatusBadRequest, "mode is required (SYSTEM_ASSIGNED or USER_SELECTED)")
		return
	}
	if strField(req, "vehicle_type") == "" {
		writeError(w, http.StatusBadRequest, "vehicle_type is required")
		return
	}
	if strField(req, "mode") == "USER_SELECTED" && strField(req, "spot_id") == "" {
		writeError(w, http.StatusBadRequest, "spot_id is required for USER_SELECTED mode")
		return
	}
	// Pass Idempotency-Key from header into request
	if key := r.Header.Get("Idempotency-Key"); key != "" {
		req["idempotency_key"] = key
	}
	var resp json.RawMessage
	if err := grpccall.Invoke(r.Context(), h.reservation, "/reservation.ReservationService/CreateReservation", req, &resp); err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *Handler) getReservation(w http.ResponseWriter, r *http.Request) {
	id := extractPathParam(r.URL.Path, "/v1/reservations/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "reservation_id required")
		return
	}
	req := map[string]string{"reservation_id": id}
	var resp json.RawMessage
	if err := grpccall.Invoke(r.Context(), h.reservation, "/reservation.ReservationService/GetReservation", req, &resp); err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) cancelReservation(w http.ResponseWriter, r *http.Request) {
	id := extractPathParam(r.URL.Path, "/v1/reservations/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "reservation_id required")
		return
	}
	req := map[string]string{"reservation_id": id}
	var resp json.RawMessage
	if err := grpccall.Invoke(r.Context(), h.reservation, "/reservation.ReservationService/CancelReservation", req, &resp); err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) holdSpot(w http.ResponseWriter, r *http.Request) {
	// Path: /v1/spots/{spot_id}/hold
	path := strings.TrimPrefix(r.URL.Path, "/v1/spots/")
	spotID := strings.TrimSuffix(path, "/hold")
	if spotID == "" || spotID == path {
		writeError(w, http.StatusBadRequest, "invalid path, expected /v1/spots/{spot_id}/hold")
		return
	}
	body, err := readBody(r)
	if err != nil {
		body = map[string]interface{}{}
	}
	if strField(body, "driver_id") == "" {
		writeError(w, http.StatusBadRequest, "driver_id is required")
		return
	}
	body["spot_id"] = spotID
	var resp json.RawMessage
	if err := grpccall.Invoke(r.Context(), h.reservation, "/reservation.ReservationService/HoldSpot", body, &resp); err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- Check-in (Presence) ---

func (h *Handler) checkIn(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strField(body, "reservation_id") == "" {
		writeError(w, http.StatusBadRequest, "reservation_id is required")
		return
	}
	if strField(body, "spot_id") == "" {
		writeError(w, http.StatusBadRequest, "spot_id is required")
		return
	}
	var resp json.RawMessage
	if err := grpccall.Invoke(r.Context(), h.presence, "/presence.PresenceService/CheckIn", body, &resp); err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- Billing ---

func (h *Handler) checkout(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strField(body, "reservation_id") == "" {
		writeError(w, http.StatusBadRequest, "reservation_id is required")
		return
	}
	if key := r.Header.Get("Idempotency-Key"); key != "" {
		body["idempotency_key"] = key
	}
	var resp json.RawMessage
	if err := grpccall.Invoke(r.Context(), h.billing, "/billing.BillingService/Checkout", body, &resp); err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- Presence ---

func (h *Handler) checkOut(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strField(body, "reservation_id") == "" {
		writeError(w, http.StatusBadRequest, "reservation_id is required")
		return
	}
	var resp json.RawMessage
	if err := grpccall.Invoke(r.Context(), h.presence, "/presence.PresenceService/CheckOut", body, &resp); err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) updateLocation(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strField(body, "reservation_id") == "" {
		writeError(w, http.StatusBadRequest, "reservation_id is required")
		return
	}
	var resp json.RawMessage
	if err := grpccall.Invoke(r.Context(), h.presence, "/presence.PresenceService/UpdateLocation", body, &resp); err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- Payment ---

func (h *Handler) getPaymentStatus(w http.ResponseWriter, r *http.Request) {
	id := extractPathParam(r.URL.Path, "/v1/payments/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "payment_id required")
		return
	}
	req := map[string]string{"payment_id": id}
	var resp json.RawMessage
	if err := grpccall.Invoke(r.Context(), h.payment, "/payment.PaymentService/GetPaymentStatus", req, &resp); err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handlePaymentPost(w http.ResponseWriter, r *http.Request) {
	// POST /v1/payments/{payment_id}/retry
	path := strings.TrimPrefix(r.URL.Path, "/v1/payments/")
	if strings.HasSuffix(path, "/retry") {
		paymentID := strings.TrimSuffix(path, "/retry")
		body, err := readBody(r)
		if err != nil {
			body = map[string]interface{}{}
		}
		body["payment_id"] = paymentID
		if key := r.Header.Get("Idempotency-Key"); key != "" {
			body["idempotency_key"] = key
		}
		var resp json.RawMessage
		if err := grpccall.Invoke(r.Context(), h.payment, "/payment.PaymentService/RetryPayment", body, &resp); err != nil {
			writeGRPCError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	writeError(w, http.StatusNotFound, "not found")
}

// --- Helpers ---

func readBody(r *http.Request) (map[string]interface{}, error) {
	defer r.Body.Close()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func extractPathParam(path, prefix string) string {
	s := strings.TrimPrefix(path, prefix)
	// Remove trailing slash or sub-paths
	if idx := strings.Index(s, "/"); idx >= 0 {
		s = s[:idx]
	}
	return s
}

func parseIntQuery(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	var n int
	for _, c := range v {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"message": msg})
}

// writeGRPCError maps gRPC status codes to HTTP status codes.
func writeGRPCError(w http.ResponseWriter, err error) {
	st, ok := status.FromError(err)
	if !ok {
		log.Error().Err(err).Msg("non-grpc error")
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	httpCode := grpcToHTTP(st.Code())
	writeJSON(w, httpCode, map[string]interface{}{
		"code":    st.Code().String(),
		"message": st.Message(),
	})
}

func grpcToHTTP(code codes.Code) int {
	switch code {
	case codes.OK:
		return http.StatusOK
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists:
		return http.StatusConflict
	case codes.FailedPrecondition:
		return http.StatusConflict
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// strField safely extracts a string field from a map.
func strField(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
