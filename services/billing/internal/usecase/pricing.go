package usecase

import (
	"encoding/json"
	"errors"
	"time"

	zen "github.com/gorules/zen-go"
	"github.com/parkir-pintar/billing/internal/model"
	"github.com/rs/zerolog/log"
)

// ErrEngineNotInitialized is returned when the JDM pricing engine is not loaded.
var ErrEngineNotInitialized = errors.New("pricing engine not initialized: JDM rules must be loaded")

// PricingEngine evaluates pricing rules using gorules/zen-go JDM engine.
// No fallback — the JDM engine must be properly initialized with valid rules.
type PricingEngine struct {
	engine   zen.Engine
	decision zen.Decision
}

// NewPricingEngine creates a PricingEngine from JDM rule content.
// Returns an error if ruleContent is empty or invalid — no silent fallback.
func NewPricingEngine(ruleContent []byte) (*PricingEngine, error) {
	if len(ruleContent) == 0 {
		return nil, errors.New("pricing rules content is empty")
	}

	engine := zen.NewEngine(zen.EngineConfig{})
	decision, err := engine.CreateDecision(ruleContent)
	if err != nil {
		engine.Dispose()
		return nil, errors.New("failed to create JDM decision from rules: " + err.Error())
	}

	log.Info().Msg("JDM pricing engine initialized")
	return &PricingEngine{
		engine:   engine,
		decision: decision,
	}, nil
}

// Dispose releases the gorules engine resources. Call on shutdown.
func (e *PricingEngine) Dispose() {
	if e.decision != nil {
		e.decision.Dispose()
	}
	if e.engine != nil {
		e.engine.Dispose()
	}
}

// Evaluate computes the PricingOutput for the given input using the JDM engine.
// Returns an error if evaluation fails — no silent fallback.
func (e *PricingEngine) Evaluate(in model.PricingInput) (model.PricingOutput, error) {
	if e.decision == nil {
		return model.PricingOutput{}, ErrEngineNotInitialized
	}

	input := map[string]any{
		"durationHours":        in.DurationHours,
		"midnightCrossings":    in.MidnightCrossings,
		"cancelElapsedMinutes": in.CancelElapsedMinutes,
		"bookingFee":           in.BookingFee,
	}

	response, err := e.decision.Evaluate(input)
	if err != nil {
		return model.PricingOutput{}, errors.New("JDM evaluation failed: " + err.Error())
	}

	// Parse the result JSON into a map
	var result map[string]any
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return model.PricingOutput{}, errors.New("failed to parse JDM result: " + err.Error())
	}

	out := model.PricingOutput{
		BookingFee:      toInt64(result["bookingFeeResult"]),
		HourlyFee:       toInt64(result["hourlyFee"]),
		OvernightFee:    toInt64(result["overnightFee"]),
		CancellationFee: toInt64(result["cancellationFee"]),
		Total:           toInt64(result["total"]),
	}

	return out, nil
}

// toInt64 safely converts a JSON number (float64) to int64.
func toInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case json.Number:
		i, _ := n.Int64()
		return i
	default:
		return 0
	}
}

// countMidnightCrossings returns the number of times a session crosses midnight.
func countMidnightCrossings(start, end time.Time) int {
	startDay := start.Truncate(24 * time.Hour)
	endDay := end.Truncate(24 * time.Hour)
	crossings := 0
	for d := startDay.Add(24 * time.Hour); !d.After(endDay); d = d.Add(24 * time.Hour) {
		crossings++
	}
	return crossings
}
