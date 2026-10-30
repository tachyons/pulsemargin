package models

import (
	"context"

	"pulsemargin/internal/checkout"
)

// DecisionTreeModel classifies checkout risk using non-linear decision splits.
type DecisionTreeModel struct {
	version string
}

// NewDecisionTreeModel initializes a tree-based ML model.
func NewDecisionTreeModel(version string) *DecisionTreeModel {
	if version == "" {
		version = "tree_ensemble_v2.0"
	}
	return &DecisionTreeModel{version: version}
}

func (m *DecisionTreeModel) Name() string {
	return m.version
}

// Predict navigates the decision tree splits to evaluate return propensity and tier.
func (m *DecisionTreeModel) Predict(ctx context.Context, features checkout.Features) (checkout.Prediction, error) {
	var tier string
	var score float64
	var confidence float64
	featureImportances := make(map[string]float64)

	// Decision paths capturing non-linear interactions:
	// Branch 1: High customer return rate
	if features.CustomerReturnRate >= 0.40 {
		tier = "HIGH_RISK"
		score = 0.88
		confidence = 0.90
		featureImportances["customer_return_rate"] = 0.70
		if features.HasBracketing {
			score = 0.94
			featureImportances["has_bracketing"] = 0.25
		}
	} else if features.HasBracketing && features.IsCOD && features.PincodeRTORate >= 0.15 {
		// Branch 2: Compound risk (Size bracketing + COD + high RTO pincode)
		tier = "HIGH_RISK"
		score = 0.86
		confidence = 0.85
		featureImportances["has_bracketing"] = 0.40
		featureImportances["is_cod"] = 0.30
		featureImportances["pincode_rto_rate"] = 0.25
	} else if features.HasBracketing && features.CartTotal >= 3500 {
		// Branch 3: High value multi-size bracketing
		tier = "HIGH_RISK"
		score = 0.82
		confidence = 0.80
		featureImportances["has_bracketing"] = 0.50
		featureImportances["cart_total"] = 0.35
	} else if features.HasBracketing {
		// Branch 4: Moderate bracketing
		tier = "MEDIUM_RISK"
		score = 0.65
		confidence = 0.75
		featureImportances["has_bracketing"] = 0.60
		if features.IsCOD {
			score = 0.72
			featureImportances["is_cod"] = 0.25
		}
	} else if features.IsCOD && features.PincodeRTORate >= 0.20 {
		// Branch 5: COD in high RTO pincode without bracketing
		tier = "MEDIUM_RISK"
		score = 0.62
		confidence = 0.70
		featureImportances["is_cod"] = 0.50
		featureImportances["pincode_rto_rate"] = 0.40
	} else if features.MaxSizeSkew >= 0.60 {
		// Branch 6: Catalog items with severe sizing skew
		tier = "MEDIUM_RISK"
		score = 0.55
		confidence = 0.65
		featureImportances["max_size_skew"] = 0.70
	} else {
		// Branch 7: Low risk normal checkout
		tier = "LOW_RISK"
		score = 0.15
		confidence = 0.90
		featureImportances["base_risk"] = 0.15
	}

	classProbabilities := map[string]float64{
		"HIGH_RISK":   0.0,
		"MEDIUM_RISK": 0.0,
		"LOW_RISK":    0.0,
	}
	classProbabilities[tier] = confidence
	for k := range classProbabilities {
		if k != tier {
			classProbabilities[k] = (1.0 - confidence) / 2.0
		}
	}

	return checkout.Prediction{
		RiskScore:          score,
		PredictedTier:      tier,
		Confidence:         confidence,
		ClassProbabilities: classProbabilities,
		FeatureWeights:     featureImportances,
		ModelVersion:       m.version,
	}, nil
}
