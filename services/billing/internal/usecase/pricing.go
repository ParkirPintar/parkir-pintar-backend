package usecase

import (
	"math"
	"time"

	"github.com/parkir-pintar/billing/internal/model"
)

// PricingEngine evaluates pricing rules. It supports an optional gorules/JDM
// rule set loaded from ruleContent. When ruleContent is nil or empty the engine
// falls back to the pure-Go evaluatePricing implementation.
type PricingEngine struct {
	// ruleContent holds the raw gorules/JDM rule bytes.
	// When non-nil the engine delegates to the gorules evaluator.
	ruleContent []byte
}

// NewPricingEngine creates a PricingEngine. If ruleContent is nil or empty the
// engine uses the built-in Go fallback for all evaluations.
func NewPricingEngine(ruleContent []byte) *PricingEngine {
	return &PricingEngine{ruleContent: ruleContent}
}

// Evaluate computes the PricingOutput for the given input. It delegates to the
// gorules engine when available, otherwise falls back to evaluatePricing.
func (e *PricingEngine) Evaluate(in model.PricingInput) model.PricingOutput {
	if len(e.ruleContent) > 0 {
		// TODO: integrate gorules/JDM engine evaluation here.
		// For now, fall through to the Go fallback.
	}
	return evaluatePricing(in)
}

// evaluatePricing is the pure-Go fallback pricing engine.
//
// Business rules:
//   - Booking fee: 5,000 IDR (passed through from input or default)
//   - Hourly rate: 5,000 IDR per started hour (ceil)
//   - Overnight fee: 20,000 IDR flat when session crosses midnight
//   - Wrong-spot penalty: 200,000 IDR
//   - No-show fee: 10,000 IDR (separate from penalty)
//   - Cancellation > 2 min: 5,000 IDR; ≤ 2 min: 0 IDR
//   - Total = booking_fee + hourly_fee + overnight_fee + penalty + noshow_fee + cancellation_fee
func evaluatePricing(in model.PricingInput) model.PricingOutput {
	out := model.PricingOutput{}

	// Booking fee: use input value if provided, otherwise default 5,000.
	if in.BookingFee > 0 {
		out.BookingFee = in.BookingFee
	} else {
		out.BookingFee = 5000
	}

	// Hourly fee: ceil(duration) * 5,000.
	if in.DurationHours > 0 {
		out.HourlyFee = int64(math.Ceil(in.DurationHours)) * 5000
	}

	// Overnight fee: flat 20,000 when crossing midnight.
	if in.CrossesMidnight {
		out.OvernightFee = 20000
	}

	// Wrong-spot penalty: 200,000.
	if in.WrongSpot {
		out.Penalty = 200000
	}

	// No-show fee: 10,000 (separate from penalty).
	if in.IsNoshow {
		out.NoshowFee = 10000
	}

	// Cancellation fee: 5,000 if elapsed > 2 min, else 0.
	if in.CancelElapsedMinutes > 2 {
		out.CancellationFee = 5000
	}

	// Total is the sum of all components.
	out.Total = out.BookingFee + out.HourlyFee + out.OvernightFee +
		out.Penalty + out.NoshowFee + out.CancellationFee

	return out
}

// crossesMidnight returns true when start and end fall on different calendar days.
func crossesMidnight(start, end time.Time) bool {
	startDay := start.Truncate(24 * time.Hour)
	endDay := end.Truncate(24 * time.Hour)
	return endDay.After(startDay)
}
