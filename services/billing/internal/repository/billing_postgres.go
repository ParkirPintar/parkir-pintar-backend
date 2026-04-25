package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parkir-pintar/billing/internal/model"
)

type billingRepo struct {
	db *pgxpool.Pool
}

func NewBillingRepository(db *pgxpool.Pool) BillingRepository {
	return &billingRepo{db: db}
}

func (r *billingRepo) Create(ctx context.Context, b *model.BillingRecord) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO billing_records (id, reservation_id, booking_fee, status, session_start)
		 VALUES ($1,$2,$3,$4,$5)`,
		b.ID, b.ReservationID, b.BookingFee, b.Status, b.SessionStart,
	)
	return err
}

func (r *billingRepo) GetByReservationID(ctx context.Context, reservationID string) (*model.BillingRecord, error) {
	var b model.BillingRecord
	err := r.db.QueryRow(ctx,
		`SELECT id, reservation_id, booking_fee, hourly_fee, overnight_fee, penalty, noshow_fee, cancellation_fee, total, status, session_start, session_end
		 FROM billing_records WHERE reservation_id=$1`, reservationID,
	).Scan(&b.ID, &b.ReservationID, &b.BookingFee, &b.HourlyFee, &b.OvernightFee,
		&b.Penalty, &b.NoshowFee, &b.CancelFee, &b.Total, &b.Status, &b.SessionStart, &b.SessionEnd)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *billingRepo) Update(ctx context.Context, b *model.BillingRecord) error {
	_, err := r.db.Exec(ctx,
		`UPDATE billing_records SET hourly_fee=$1, overnight_fee=$2, penalty=$3, total=$4, status=$5, session_end=$6 WHERE id=$7`,
		b.HourlyFee, b.OvernightFee, b.Penalty, b.Total, b.Status, b.SessionEnd, b.ID,
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
