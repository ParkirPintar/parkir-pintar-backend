package usecase

import (
	"encoding/json"
	"math"
	"os"
	"time"

	zen "github.com/gorules/zen-go"
	"github.com/parkir-pintar/billing/internal/model"
	"github.com/rs/zerolog/log"
)

// getNoshowFeeFromRules reads the no-show fee from the gorules pricing JSON file.
// This keeps pricing.json as the single source of truth for all fee values.
// Falls back to 5000 IDR if the file cannot be read or parsed.
func getNoshowFeeFromRules() int64 {
	rulesPath := os.Getenv("PRICING_RULES_PATH")
	if rulesPath == "" {
		rulesPath = "rules/pricing.json"
	}

	data, err := os.ReadFile(rulesPath)
	if err != nil {
		return 5000
	}

	var rules struct {
		Nodes []struct {
			ID      string `json:"id"`
			Content struct {
				Rules []map[string]string `json:"rules"`
			} `json:"content"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(data, &rules); err != nil {
		return 5000
	}

	for _, node := range rules.Nodes {
		if node.ID == "noshow-fee-table" {
			for _, rule := range node.Content.Rules {
				if val, ok := rule["ns_o1"]; ok && val != "0" {
					var fee int64
					if err := json.Unmarshal([]byte(val), &fee); err == nil && fee > 0 {
						return fee
					}
				}
			}
		}
	}

	return 5000
}

// PricingEngine evaluates pricing rules using gorules/zen-go JDM engine.
// When ruleContent is provided, it delegates to the JDM engine.
// When ruleContent is nil or evaluation fails, it falls back to pure-Go logic.
type PricingEngine struct {
	engine   zen.Engine
	decision zen.Decision
}

// NewPricingEngine creates a PricingEngine. If ruleContent is valid JDM JSON,
// the engine uses gorules for evaluation. Otherwise falls back to Go.
func NewPricingEngine(ruleContent []byte) *PricingEngine {
	pe := &PricingEngine{}

	if len(ruleContent) > 0 {
		engine := zen.NewEngine(zen.EngineConfig{})
		decision, err := engine.CreateDecision(ruleContent)
		if err != nil {
			log.Error().Err(err).Msg("failed to create JDM decision, using Go fallback")
			engine.Dispose()
			return pe
		}
		pe.engine = engine
		pe.decision = decision
		log.Info().Msg("JDM pricing engine initialized")
	}

	return pe
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

// Evaluate computes the PricingOutput for the given input.
// Delegates to JDM engine when available, falls back to pure-Go otherwise.
func (e *PricingEngine) Evaluate(in model.PricingInput) model.PricingOutput {
	if e.decision != nil {
		result, err := e.evaluateJDM(in)
		if err != nil {
			log.Error().Err(err).Msg("JDM evaluation failed, falling back to Go")
			return evaluatePricing(in)
		}
		return result
	}
	return evaluatePricing(in)
}

// evaluateJDM runs the input through the gorules JDM decision graph.
func (e *PricingEngine) evaluateJDM(in model.PricingInput) (model.PricingOutput, error) {
	input := map[string]any{
		"durationHours":        in.DurationHours,
		"midnightCrossings":    in.MidnightCrossings,
		"isNoshow":             in.IsNoshow,
		"cancelElapsedMinutes": in.CancelElapsedMinutes,
		"bookingFee":           in.BookingFee,
	}

	response, err := e.decision.Evaluate(input)
	if err != nil {
		return model.PricingOutput{}, err
	}

	// Parse the result JSON into a map
	var result map[string]any
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return model.PricingOutput{}, err
	}

	out := model.PricingOutput{
		BookingFee:      toInt64(result["bookingFeeResult"]),
		HourlyFee:       toInt64(result["hourlyFee"]),
		OvernightFee:    toInt64(result["overnightFee"]),
		NoshowFee:       toInt64(result["noshowFee"]),
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

// evaluatePricing is the pure-Go fallback pricing engine.
// Used when JDM rules are not loaded or evaluation fails.
//
// Business rules:
//   - Booking fee: 5,000 IDR (passed through from input or default)
//   - Hourly rate: 5,000 IDR per started hour (ceil)
//   - Overnight fee: 20,000 IDR per midnight crossing (cumulative)
//   - No-show fee: read from gorules pricing.json (default 5,000 IDR)
//   - Cancellation > 2 min: 5,000 IDR; ≤ 2 min: 0 IDR
//   - Total = booking_fee + hourly_fee + overnight_fee + noshow_fee + cancellation_fee
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

	// Overnight fee: 20,000 per midnight crossing (cumulative).
	if in.MidnightCrossings > 0 {
		out.OvernightFee = int64(in.MidnightCrossings) * 20000
	}

	// No-show fee: read from gorules pricing.json (single source of truth).
	if in.IsNoshow {
		out.NoshowFee = getNoshowFeeFromRules()
	}

	// Cancellation fee: 5,000 if elapsed > 2 min, else 0.
	if in.CancelElapsedMinutes > 2 {
		out.CancellationFee = 5000
	}

	// Total is the sum of all components.
	out.Total = out.BookingFee + out.HourlyFee + out.OvernightFee +
		out.NoshowFee + out.CancellationFee

	return out
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
