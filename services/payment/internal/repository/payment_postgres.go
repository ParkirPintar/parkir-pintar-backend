package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parkir-pintar/payment/internal/model"
)

type paymentRepo struct {
	db *pgxpool.Pool
}

func NewPaymentRepository(db *pgxpool.Pool) PaymentRepository {
	return &paymentRepo{db: db}
}

func (r *paymentRepo) Create(ctx context.Context, p *model.Payment) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO payments (id, invoice_id, amount, status, method, qr_code, idempotency_key, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		p.ID, p.InvoiceID, p.Amount, p.Status, p.Method, p.QRCode, p.IdempotencyKey, time.Now(),
	)
	return err
}

func (r *paymentRepo) GetByID(ctx context.Context, id string) (*model.Payment, error) {
	var p model.Payment
	err := r.db.QueryRow(ctx,
		`SELECT id, invoice_id, amount, status, method, qr_code, idempotency_key, updated_at FROM payments WHERE id=$1`, id,
	).Scan(&p.ID, &p.InvoiceID, &p.Amount, &p.Status, &p.Method, &p.QRCode, &p.IdempotencyKey, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *paymentRepo) UpdateStatus(ctx context.Context, id string, status model.PaymentStatus) error {
	_, err := r.db.Exec(ctx, `UPDATE payments SET status=$1, updated_at=$2 WHERE id=$3`, status, time.Now(), id)
	return err
}
