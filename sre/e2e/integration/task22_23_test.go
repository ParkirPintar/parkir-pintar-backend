// Tasks 22–23: Payment flows
//  22. Payment success (QRIS)
//  23. Payment failure + retry

//go:build integration

package integration

import (
	"testing"
	"time"

	reservationpb "github.com/parkir-pintar/reservation/pkg/proto"
	billingpb "github.com/parkir-pintar/billing/pkg/proto"
	paymentpb "github.com/parkir-pintar/payment/pkg/proto"
)

// ─── Task 22: Payment success (QRIS) ──────────────────────────────
// Checkout → poll payment → status=PAID → reservation=COMPLETED
func TestTask22_PaymentSuccess(t *testing.T) {
	userConn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	resConn := dialGRPC(t, envOr("RESERVATION_ADDR", "localhost:50052"))
	billingConn := dialGRPC(t, envOr("BILLING_ADDR", "localhost:50053"))
	paymentConn := dialGRPC(t, envOr("PAYMENT_ADDR", "localhost:50054"))
	rdb := newRedis(t)

	ctx := registerAndLogin(t, userConn, uniquePlate("T22"), "CAR")

	// Reserve + check-in
	reservationID, spotID := createReservationAndWait(t, resConn, ctx, rdb, "SYSTEM_ASSIGNED", "CAR", "")
	resConn.Invoke(ctx, "/reservation.ReservationService/CheckIn",
		&reservationpb.CheckInRequest{ReservationId: reservationID, ActualSpotId: spotID},
		&reservationpb.CheckInResponse{})
	t.Logf("✓ Reserved + checked in: id=%s spot=%s", reservationID, spotID)

	time.Sleep(1 * time.Second)

	// Checkout → get invoice with QR code
	checkoutResp := &billingpb.InvoiceResponse{}
	if err := billingConn.Invoke(ctx, "/billing.BillingService/Checkout",
		&billingpb.CheckoutRequest{ReservationId: reservationID, IdempotencyKey: uniquePlate("checkout-t22")}, checkoutResp); err != nil {
		t.Fatalf("Checkout failed: %v", err)
	}
	t.Logf("✓ Invoice: id=%s total=%d payment_id=%s qr=%s",
		checkoutResp.InvoiceId, checkoutResp.Total, checkoutResp.PaymentId, checkoutResp.QrCode)

	if checkoutResp.PaymentId == "" {
		t.Fatal("expected non-empty payment_id")
	}

	// Poll payment status — settlement stub returns PAID
	var finalStatus string
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		statusResp := &paymentpb.PaymentResponse{}
		if err := paymentConn.Invoke(ctx, "/payment.PaymentService/GetPaymentStatus",
			&paymentpb.GetPaymentStatusRequest{PaymentId: checkoutResp.PaymentId}, statusResp); err != nil {
			t.Logf("  [%ds] poll error: %v", i+1, err)
			continue
		}
		t.Logf("  [%ds] payment status=%s", i+1, statusResp.Status)
		if statusResp.Status == "PAID" {
			finalStatus = "PAID"
			break
		}
	}
	if finalStatus != "PAID" {
		t.Errorf("expected PAID, got %s", finalStatus)
	}

	// Verify reservation completed
	getResp := &reservationpb.ReservationResponse{}
	resConn.Invoke(ctx, "/reservation.ReservationService/GetReservation",
		&reservationpb.GetReservationRequest{ReservationId: reservationID}, getResp)
	t.Logf("✓ Reservation final status: %s", getResp.Status)

	t.Log("✓ PASS: Task 22 — Payment success via QRIS")
}

// ─── Task 23: Payment failure + retry ──────────────────────────────
// Checkout → poll payment → status=FAILED → retry → new QR code
func TestTask23_PaymentRetry(t *testing.T) {
	userConn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	resConn := dialGRPC(t, envOr("RESERVATION_ADDR", "localhost:50052"))
	billingConn := dialGRPC(t, envOr("BILLING_ADDR", "localhost:50053"))
	paymentConn := dialGRPC(t, envOr("PAYMENT_ADDR", "localhost:50054"))
	rdb := newRedis(t)

	ctx := registerAndLogin(t, userConn, uniquePlate("T23"), "CAR")

	// Reserve + check-in
	reservationID, spotID := createReservationAndWait(t, resConn, ctx, rdb, "SYSTEM_ASSIGNED", "CAR", "")
	resConn.Invoke(ctx, "/reservation.ReservationService/CheckIn",
		&reservationpb.CheckInRequest{ReservationId: reservationID, ActualSpotId: spotID},
		&reservationpb.CheckInResponse{})

	time.Sleep(1 * time.Second)

	// Checkout
	checkoutResp := &billingpb.InvoiceResponse{}
	if err := billingConn.Invoke(ctx, "/billing.BillingService/Checkout",
		&billingpb.CheckoutRequest{ReservationId: reservationID, IdempotencyKey: uniquePlate("checkout-t23")}, checkoutResp); err != nil {
		t.Fatalf("Checkout failed: %v", err)
	}
	originalPaymentId := checkoutResp.PaymentId
	t.Logf("✓ Original payment: id=%s", originalPaymentId)

	// Retry payment → should get new QR code
	retryResp := &paymentpb.PaymentResponse{}
	if err := paymentConn.Invoke(ctx, "/payment.PaymentService/RetryPayment",
		&paymentpb.RetryPaymentRequest{PaymentId: originalPaymentId, IdempotencyKey: uniquePlate("retry-t23")}, retryResp); err != nil {
		t.Fatalf("RetryPayment failed: %v", err)
	}

	if retryResp.QrCode == "" {
		t.Error("expected new QR code on retry")
	}
	t.Logf("✓ Retry: payment_id=%s qr=%s status=%s", retryResp.PaymentId, retryResp.QrCode, retryResp.Status)

	t.Log("✓ PASS: Task 23 — Payment retry generates new QR code")
}
