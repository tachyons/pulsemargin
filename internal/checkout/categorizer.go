package checkout

import (
	"context"
	"strings"
)

// MLCategorizer determines the action tier and specific interventions driven by ML model predictions and feature importances.
type MLCategorizer struct{}

// NewMLCategorizer creates an ML-driven categorizer.
func NewMLCategorizer() *MLCategorizer {
	return &MLCategorizer{}
}

// Categorize maps the model prediction and checkout context into an actionable decision tier and targeted interventions.
func (c *MLCategorizer) Categorize(ctx context.Context, payload Payload, prediction Prediction) (string, []string, error) {
	tier := prediction.PredictedTier
	if tier == "" {
		// Fallback to score-based tiering if model did not specify tier
		switch {
		case prediction.RiskScore >= 0.80:
			tier = "HIGH_RISK"
		case prediction.RiskScore >= 0.50:
			tier = "MEDIUM_RISK"
		default:
			tier = "LOW_RISK"
		}
	}

	interventions := make([]string, 0)

	switch tier {
	case "HIGH_RISK":
		// Analyze ML feature contributions to provide targeted, dynamic interventions
		hasBracketingFactor := prediction.FeatureWeights["multi_size_detected"] > 0 ||
			prediction.FeatureWeights["has_bracketing"] > 0
		hasCODFactor := prediction.FeatureWeights["is_cod"] > 0 ||
			strings.EqualFold(payload.PaymentMode, "COD")
		hasPinFactor := prediction.FeatureWeights["pincode_rto_rate"] > 0.20
		hasCustFactor := prediction.FeatureWeights["customer_return_rate"] > 0.30

		if hasBracketingFactor {
			interventions = append(interventions, "Trigger conversational WhatsApp sizing verification prior to fulfillment")
		}

		if hasCODFactor || hasPinFactor {
			interventions = append(interventions,
				"Restrict COD: Offer ₹100 instant discount for prepaid conversion",
				"Route order to surface courier tier to minimize air-express return loss",
			)
		}

		if hasCustFactor {
			interventions = append(interventions, "Require one-time password (OTP) verification upon delivery")
		}

		// Ensure high risk always has at least baseline interventions
		if len(interventions) == 0 {
			interventions = append(interventions,
				"Trigger conversational WhatsApp sizing verification prior to fulfillment",
				"Restrict COD: Offer ₹100 instant discount for prepaid conversion",
				"Route order to surface courier tier to minimize air-express return loss",
			)
		}

	case "MEDIUM_RISK":
		hasBracketingFactor := prediction.FeatureWeights["multi_size_detected"] > 0 ||
			prediction.FeatureWeights["has_bracketing"] > 0
		hasSkewFactor := prediction.FeatureWeights["size_skew"] > 0 ||
			prediction.FeatureWeights["max_size_skew"] > 0

		if hasBracketingFactor || hasSkewFactor {
			interventions = append(interventions, "Send automated sizing chart verification prompt via SMS/Email")
		}

		interventions = append(interventions, "Offer 5% non-refundable store credit coupon at delivery confirmation")

	default: // LOW_RISK
		interventions = append(interventions, "Standard Fast-Track Fulfillment")
	}

	return tier, interventions, nil
}

// RuleBasedCategorizer classifies return risk using static threshold rules (the legacy approach).
type RuleBasedCategorizer struct {
	HighRiskThreshold   float64
	MediumRiskThreshold float64
}

// NewRuleBasedCategorizer creates a categorizer that relies on static numeric thresholds.
func NewRuleBasedCategorizer() *RuleBasedCategorizer {
	return &RuleBasedCategorizer{
		HighRiskThreshold:   0.80,
		MediumRiskThreshold: 0.50,
	}
}

// Categorize executes static boundary rules.
func (r *RuleBasedCategorizer) Categorize(ctx context.Context, payload Payload, prediction Prediction) (string, []string, error) {
	var tier string
	var interventions []string

	switch {
	case prediction.RiskScore >= r.HighRiskThreshold:
		tier = "HIGH_RISK"
		interventions = []string{
			"Trigger conversational WhatsApp sizing verification prior to fulfillment",
			"Restrict COD: Offer ₹100 instant discount for prepaid conversion",
			"Route order to surface courier tier to minimize air-express return loss",
		}
	case prediction.RiskScore >= r.MediumRiskThreshold:
		tier = "MEDIUM_RISK"
		interventions = []string{
			"Send automated sizing chart verification prompt via SMS/Email",
			"Offer 5% non-refundable store credit coupon at delivery confirmation",
		}
	default:
		tier = "LOW_RISK"
		interventions = []string{
			"Standard Fast-Track Fulfillment",
		}
	}

	return tier, interventions, nil
}
