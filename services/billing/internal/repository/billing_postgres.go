package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/parkir-pintar/billing/internal/model"
)

type billingRepo struct {
	db  *pgxpool.Pool
	rdb *redis.Client
}

// NewBillingRepository creates a new BillingRepository backed by PostgreSQL and Redis.
// Redis is used for checkout idempotency keys with 24h TTL.
func NewBillingRepository(db *pgxpool.Pool, rdb *redis.Client) BillingRepository {
	return &billingRepo{db: db, rdb: rdb}
}

func (r *billingRepo) Create(ctx context.Context, b *model.BillingRecord) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO billing_records (id, reservation_id, booking_fee, status, session_start, idempotency_key, payment_id, qr_code)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		b.ID, b.ReservationID, b.BookingFee, b.Status, b.SessionStart,
		nilIfEmpty(b.IdempotencyKey), nilIfEmpty(b.PaymentID), nilIfEmpty(b.QRCode),
	)
	return err
}

func (r *billingRepo) GetByReservationID(ctx context.Context, reservationID string) (*model.BillingRecord, error) {
	var b model.BillingRecord
	var idempotencyKey, paymentID, qrCode *string
	err := r.db.QueryRow(ctx,
		`SELECT id, reservation_id, booking_fee, hourly_fee, overnight_fee, penalty, noshow_fee, cancellation_fee, total, status, session_start, session_end, idempotency_key, payment_id, qr_code
		 FROM billing_records WHERE reservation_id=$1`, reservationID,
	).Scan(&b.ID, &b.ReservationID, &b.BookingFee, &b.HourlyFee, &b.OvernightFee,
		&b.Penalty, &b.NoshowFee, &b.CancelFee, &b.Total, &b.Status, &b.SessionStart, &b.SessionEnd,
		&idempotencyKey, &paymentID, &qrCode)
	if err != nil {
		return nil, err
	}
	if idempotencyKey != nil {
		b.IdempotencyKey = *idempotencyKey
	}
	if paymentID != nil {
		b.PaymentID = *paymentID
	}
	if qrCode != nil {
		b.QRCode = *qrCode
	}
	return &b, nil
}

func (r *billingRepo) Update(ctx context.Context, b *model.BillingRecord) error {
	_, err := r.db.Exec(ctx,
		`UPDATE billing_records
		 SET hourly_fee=$1, overnight_fee=$2, penalty=$3, noshow_fee=$4, cancellation_fee=$5,
		     total=$6, status=$7, session_end=$8, idempotency_key=$9, payment_id=$10, qr_code=$11
		 WHERE id=$12`,
		b.HourlyFee, b.OvernightFee, b.Penalty, b.NoshowFee, b.CancelFee,
		b.Total, b.Status, b.SessionEnd,
		nilIfEmpty(b.IdempotencyKey), nilIfEmpty(b.PaymentID), nilIfEmpty(b.QRCode),
		b.ID,
	)
	return err
}

func (r *billingRepo) GetActivePricingRule(ctx context.Context) ([]byte, int, error) {
	var content []byte
	var version int
	err := r.db.QueryRow(ctx,
		`SELECT content, version FROM pricing_rules WHERE is_active=true ORDER BY version DESC LIMIT 1`,
	).Scan(&content, &version)
	return content, version, err
}

// GetByIdempotencyKey checks Redis first for a cached invoice_id, then falls back
// to a database lookup. Returns nil, nil if no record is found.
func (r *billingRepo) GetByIdempotencyKey(ctx context.Context, key string) (*model.BillingRecord, error) {
	redisKey := fmt.Sprintf("idempotency:checkout:%s", key)

	// Check Redis first for fast lookup.
	invoiceID, err := r.rdb.Get(ctx, redisKey).Result()
	if err == nil && invoiceID != "" {
		// Found in Redis — fetch the full record from DB by ID.
		return r.getByID(ctx, invoiceID)
	}
	if err != nil && err != redis.Nil {
		// Redis error — fall through to DB lookup.
	}

	// Fall back to database lookup by idempotency_key column.
	var b model.BillingRecord
	var idempotencyKey, paymentID, qrCode *string
	err = r.db.QueryRow(ctx,
		`SELECT id, reservation_id, booking_fee, hourly_fee, overnight_fee, penalty, noshow_fee, cancellation_fee, total, status, session_start, session_end, idempotency_key, payment_id, qr_code
		 FROM billing_records WHERE idempotency_key=$1`, key,
	).Scan(&b.ID, &b.ReservationID, &b.BookingFee, &b.HourlyFee, &b.OvernightFee,
		&b.Penalty, &b.NoshowFee, &b.CancelFee, &b.Total, &b.Status, &b.SessionStart, &b.SessionEnd,
		&idempotencyKey, &paymentID, &qrCode)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if idempotencyKey != nil {
		b.IdempotencyKey = *idempotencyKey
	}
	if paymentID != nil {
		b.PaymentID = *paymentID
	}
	if qrCode != nil {
		b.QRCode = *qrCode
	}
	return &b, nil
}

// SetIdempotencyKey stores the checkout idempotency key in Redis with a 24-hour TTL.
// The key maps to the invoice (billing record) ID for fast dedup lookups.
func (r *billingRepo) SetIdempotencyKey(ctx context.Context, key string, invoiceID string) error {
	redisKey := fmt.Sprintf("idempotency:checkout:%s", key)
	return r.rdb.Set(ctx, redisKey, invoiceID, 24*time.Hour).Err()
}

// getByID fetches a billing record by its primary key.
func (r *billingRepo) getByID(ctx context.Context, id string) (*model.BillingRecord, error) {
	var b model.BillingRecord
	var idempotencyKey, paymentID, qrCode *string
	err := r.db.QueryRow(ctx,
		`SELECT id, reservation_id, booking_fee, hourly_fee, overnight_fee, penalty, noshow_fee, cancellation_fee, total, status, session_start, session_end, idempotency_key, payment_id, qr_code
		 FROM billing_records WHERE id=$1`, id,
	).Scan(&b.ID, &b.ReservationID, &b.BookingFee, &b.HourlyFee, &b.OvernightFee,
		&b.Penalty, &b.NoshowFee, &b.CancelFee, &b.Total, &b.Status, &b.SessionStart, &b.SessionEnd,
		&idempotencyKey, &paymentID, &qrCode)
	if err != nil {
		return nil, err
	}
	if idempotencyKey != nil {
		b.IdempotencyKey = *idempotencyKey
	}
	if paymentID != nil {
		b.PaymentID = *paymentID
	}
	if qrCode != nil {
		b.QRCode = *qrCode
	}
	return &b, nil
}

// nilIfEmpty returns nil for empty strings, allowing NULL storage in PostgreSQL.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
