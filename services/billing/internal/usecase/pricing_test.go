package usecase

import (
	"os"
	"testing"
	"time"

	"github.com/parkir-pintar/billing/internal/model"
)

func loadJDMRules(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../rules/pricing.json")
	if err != nil {
		t.Fatalf("JDM rules file not found — gorules is mandatory: %v", err)
	}
	return data
}

func TestNewPricingEngine_EmptyRules_ReturnsError(t *testing.T) {
	_, err := NewPricingEngine(nil)
	if err == nil {
		t.Fatal("expected error when creating engine with nil rules, got nil")
	}

	_, err = NewPricingEngine([]byte{})
	if err == nil {
		t.Fatal("expected error when creating engine with empty rules, got nil")
	}
}

func TestNewPricingEngine_InvalidRules_ReturnsError(t *testing.T) {
	_, err := NewPricingEngine([]byte("not valid json"))
	if err == nil {
		t.Fatal("expected error when creating engine with invalid rules, got nil")
	}
}

func TestPricingEngine_JDM_HourlyFee(t *testing.T) {
	rules := loadJDMRules(t)
	engine, err := NewPricingEngine(rules)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer engine.Dispose()

	out, err := engine.Evaluate(model.PricingInput{
		DurationHours: 3,
		BookingFee:    5000,
	})
	if err != nil {
		t.Fatalf("evaluate failed: %v", err)
	}

	if out.BookingFee != 5000 {
		t.Errorf("booking_fee = %d, want 5000", out.BookingFee)
	}
	if out.HourlyFee != 15000 {
		t.Errorf("hourly_fee = %d, want 15000 (3h * 5000)", out.HourlyFee)
	}
	if out.Total != 20000 {
		t.Errorf("total = %d, want 20000", out.Total)
	}
}

func TestPricingEngine_JDM_OvernightFee(t *testing.T) {
	rules := loadJDMRules(t)
	engine, err := NewPricingEngine(rules)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer engine.Dispose()

	out, err := engine.Evaluate(model.PricingInput{
		DurationHours:     10,
		MidnightCrossings: 1,
		BookingFee:        5000,
	})
	if err != nil {
		t.Fatalf("evaluate failed: %v", err)
	}

	if out.OvernightFee != 20000 {
		t.Errorf("overnight_fee = %d, want 20000", out.OvernightFee)
	}
	expectedTotal := int64(5000 + 50000 + 20000) // booking + 10h*5000 + overnight
	if out.Total != expectedTotal {
		t.Errorf("total = %d, want %d", out.Total, expectedTotal)
	}
}

func TestPricingEngine_JDM_TwoNightsOvernightFee(t *testing.T) {
	rules := loadJDMRules(t)
	engine, err := NewPricingEngine(rules)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer engine.Dispose()

	out, err := engine.Evaluate(model.PricingInput{
		DurationHours:     30,
		MidnightCrossings: 2,
		BookingFee:        5000,
	})
	if err != nil {
		t.Fatalf("evaluate failed: %v", err)
	}

	if out.OvernightFee != 40000 {
		t.Errorf("overnight_fee = %d, want 40000 (2 crossings * 20000)", out.OvernightFee)
	}
}

func TestPricingEngine_JDM_CancellationFreeUnder2Min(t *testing.T) {
	rules := loadJDMRules(t)
	engine, err := NewPricingEngine(rules)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer engine.Dispose()

	out, err := engine.Evaluate(model.PricingInput{
		CancelElapsedMinutes: 1.5,
		BookingFee:           5000,
	})
	if err != nil {
		t.Fatalf("evaluate failed: %v", err)
	}

	if out.CancellationFee != 0 {
		t.Errorf("cancellation_fee = %d, want 0 (under 2 min)", out.CancellationFee)
	}
}

func TestPricingEngine_JDM_CancellationFeeOver2Min(t *testing.T) {
	rules := loadJDMRules(t)
	engine, err := NewPricingEngine(rules)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer engine.Dispose()

	out, err := engine.Evaluate(model.PricingInput{
		CancelElapsedMinutes: 5,
		BookingFee:           5000,
	})
	if err != nil {
		t.Fatalf("evaluate failed: %v", err)
	}

	if out.CancellationFee != 5000 {
		t.Errorf("cancellation_fee = %d, want 5000", out.CancellationFee)
	}
}

func TestCountMidnightCrossings(t *testing.T) {
	tests := []struct {
		name     string
		startH   int
		endH     int
		addDays  int
		expected int
	}{
		{"same day", 8, 17, 0, 0},
		{"crosses midnight once", 22, 2, 1, 1},
		{"two nights", 22, 6, 2, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Date(2024, 1, 15, tt.startH, 0, 0, 0, time.UTC)
			end := time.Date(2024, 1, 15+tt.addDays, tt.endH, 0, 0, 0, time.UTC)
			got := countMidnightCrossings(start, end)
			if got != tt.expected {
				t.Errorf("countMidnightCrossings = %d, want %d", got, tt.expected)
			}
		})
	}
}
