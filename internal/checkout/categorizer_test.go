package checkout

import (
	"context"
	"testing"
)

func TestMLCategorizer(t *testing.T) {
	categorizer := NewMLCategorizer()
	ctx := context.Background()

	t.Run("high risk with bracketing and COD factors", func(t *testing.T) {
		payload := Payload{
			PaymentMode: "COD",
		}
		pred := Prediction{
			RiskScore:     0.85,
			PredictedTier: "HIGH_RISK",
			FeatureWeights: map[string]float64{
				"multi_size_detected": 1.95,
				"is_cod":              1.10,
				"pincode_rto_rate":    0.60,
			},
		}

		tier, interventions, err := categorizer.Categorize(ctx, payload, pred)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tier != "HIGH_RISK" {
			t.Errorf("tier = %s, want HIGH_RISK", tier)
		}

		hasWhatsApp := false
		hasCOD := false
		for _, inv := range interventions {
			if inv == "Trigger conversational WhatsApp sizing verification prior to fulfillment" {
				hasWhatsApp = true
			}
			if inv == "Restrict COD: Offer ₹100 instant discount for prepaid conversion" {
				hasCOD = true
			}
		}

		if !hasWhatsApp {
			t.Errorf("expected WhatsApp sizing verification for bracketing risk")
		}
		if !hasCOD {
			t.Errorf("expected COD restriction for COD risk")
		}
	})

	t.Run("medium risk with sizing skew", func(t *testing.T) {
		payload := Payload{PaymentMode: "PREPAID"}
		pred := Prediction{
			RiskScore:     0.60,
			PredictedTier: "MEDIUM_RISK",
			FeatureWeights: map[string]float64{
				"size_skew": 1.2,
			},
		}

		tier, interventions, err := categorizer.Categorize(ctx, payload, pred)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tier != "MEDIUM_RISK" {
			t.Errorf("tier = %s, want MEDIUM_RISK", tier)
		}
		if len(interventions) == 0 {
			t.Errorf("expected interventions for medium risk")
		}
	})

	t.Run("low risk clean order", func(t *testing.T) {
		payload := Payload{PaymentMode: "PREPAID"}
		pred := Prediction{
			RiskScore:     0.15,
			PredictedTier: "LOW_RISK",
		}

		tier, interventions, err := categorizer.Categorize(ctx, payload, pred)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tier != "LOW_RISK" {
			t.Errorf("tier = %s, want LOW_RISK", tier)
		}
		if len(interventions) != 1 || interventions[0] != "Standard Fast-Track Fulfillment" {
			t.Errorf("unexpected interventions: %v", interventions)
		}
	})
}

func TestRuleBasedCategorizer(t *testing.T) {
	categorizer := NewRuleBasedCategorizer()
	ctx := context.Background()
	payload := Payload{}

	tests := []struct {
		score    float64
		wantTier string
	}{
		{0.95, "HIGH_RISK"},
		{0.80, "HIGH_RISK"},
		{0.79, "MEDIUM_RISK"},
		{0.50, "MEDIUM_RISK"},
		{0.49, "LOW_RISK"},
		{0.10, "LOW_RISK"},
	}

	for _, tt := range tests {
		tier, interventions, err := categorizer.Categorize(ctx, payload, Prediction{RiskScore: tt.score})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tier != tt.wantTier {
			t.Errorf("score %f: tier = %s, want %s", tt.score, tier, tt.wantTier)
		}
		if len(interventions) == 0 {
			t.Errorf("expected non-empty interventions for score %f", tt.score)
		}
	}
}
