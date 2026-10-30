package models

import (
	"context"
	"math"

	"pulsemargin/internal/checkout"
)

// LogisticRegressionWeights holds the linear model coefficients.
type LogisticRegressionWeights struct {
	Intercept         float64 `json:"intercept"`
	CustReturnRate    float64 `json:"cust_return_rate"`
	PincodeRTORate    float64 `json:"pincode_rto_rate"`
	MultiSizeDetected float64 `json:"multi_size_detected"`
	SizeSkew          float64 `json:"size_skew"`
	IsCOD             float64 `json:"is_cod"`
	CartValueNorm     float64 `json:"cart_value_norm"`
}

// DefaultLogisticWeights provides the calibrated model weights.
func DefaultLogisticWeights() LogisticRegressionWeights {
	return LogisticRegressionWeights{
		Intercept:         -2.15,
		CustReturnRate:    3.80,
		PincodeRTORate:    2.40,
		MultiSizeDetected: 1.95,
		SizeSkew:          1.20,
		IsCOD:             1.10,
		CartValueNorm:     0.0002,
	}
}

// LogisticRegressionModel evaluates return risk using a calibrated logistic regression model.
type LogisticRegressionModel struct {
	version string
	weights LogisticRegressionWeights
}

// NewLogisticRegressionModel initializes the logistic regression ML model.
func NewLogisticRegressionModel(version string, weights LogisticRegressionWeights) *LogisticRegressionModel {
	if version == "" {
		version = "logistic_regression_v1.2"
	}
	return &LogisticRegressionModel{
		version: version,
		weights: weights,
	}
}

func (m *LogisticRegressionModel) Name() string {
	return m.version
}

// Predict calculates logit, probability via sigmoid activation, and feature contributions.
func (m *LogisticRegressionModel) Predict(ctx context.Context, features checkout.Features) (checkout.Prediction, error) {
	w := m.weights

	bracketVal := 0.0
	if features.HasBracketing {
		bracketVal = 1.0
	}

	codVal := 0.0
	if features.IsCOD {
		codVal = 1.0
	}

	termCust := w.CustReturnRate * features.CustomerReturnRate
	termPin := w.PincodeRTORate * features.PincodeRTORate
	termBracket := w.MultiSizeDetected * bracketVal
	termSkew := w.SizeSkew * features.MaxSizeSkew
	termCOD := w.IsCOD * codVal
	termValue := w.CartValueNorm * features.CartTotal

	logit := w.Intercept + termCust + termPin + termBracket + termSkew + termCOD + termValue
	score := 1.0 / (1.0 + math.Exp(-logit))

	// Clamp to [0.0, 1.0]
	if score < 0.0 {
		score = 0.0
	} else if score > 1.0 {
		score = 1.0
	}

	// Model predicted tier based on model decision boundaries
	var tier string
	switch {
	case score >= 0.80:
		tier = "HIGH_RISK"
	case score >= 0.50:
		tier = "MEDIUM_RISK"
	default:
		tier = "LOW_RISK"
	}

	// Prediction confidence: distance from decision threshold 0.50
	confidence := math.Min(1.0, math.Abs(score-0.5)*2.0)

	featureWeights := map[string]float64{
		"intercept":            w.Intercept,
		"customer_return_rate": termCust,
		"pincode_rto_rate":     termPin,
		"multi_size_detected":  termBracket,
		"size_skew":            termSkew,
		"is_cod":               termCOD,
		"cart_value":           termValue,
	}

	classProbabilities := map[string]float64{
		"HIGH_RISK":   math.Max(0, (score-0.5)*2.0),
		"MEDIUM_RISK": 1.0 - math.Abs(score-0.5)*2.0,
		"LOW_RISK":    math.Max(0, (0.5-score)*2.0),
	}

	return checkout.Prediction{
		RiskScore:          score,
		PredictedTier:      tier,
		Confidence:         confidence,
		ClassProbabilities: classProbabilities,
		FeatureWeights:     featureWeights,
		ModelVersion:       m.version,
	}, nil
}
