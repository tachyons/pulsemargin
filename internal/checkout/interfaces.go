package checkout

import (
	"context"
)

// Scorer evaluates checkout return risk and returns an actionable decision.
type Scorer interface {
	Score(ctx context.Context, payload Payload) (Decision, error)
}

// Features represents the feature vector extracted for machine learning scoring and categorization.
type Features struct {
	CartTotal          float64            `json:"cart_total"`
	ItemCount          int                `json:"item_count"`
	HasBracketing      bool               `json:"has_bracketing"`
	IsCOD              bool               `json:"is_cod"`
	CustomerReturnRate float64            `json:"customer_return_rate"`
	PincodeRTORate     float64            `json:"pincode_rto_rate"`
	MaxSizeSkew        float64            `json:"max_size_skew"`
	Categories         []string           `json:"categories"`
	CustomAttributes   map[string]float64 `json:"custom_attributes,omitempty"`
}

// Prediction captures the output of a machine learning model inference.
type Prediction struct {
	RiskScore          float64            `json:"risk_score"`
	PredictedTier      string             `json:"predicted_tier"`
	Confidence         float64            `json:"confidence"`
	ClassProbabilities map[string]float64 `json:"class_probabilities,omitempty"`
	FeatureWeights     map[string]float64 `json:"feature_weights,omitempty"`
	ModelVersion       string             `json:"model_version"`
}

// MLModel defines the interface for machine learning models that predict return risk and categorize checkouts.
type MLModel interface {
	// Name returns the model identifier and version.
	Name() string
	// Predict executes model inference against the input features.
	Predict(ctx context.Context, features Features) (Prediction, error)
}

// Categorizer defines how a prediction is mapped into actionable interception tiers and interventions.
type Categorizer interface {
	// Categorize determines the action tier and recommended interventions from model prediction and payload context.
	Categorize(ctx context.Context, payload Payload, prediction Prediction) (tier string, interventions []string, err error)
}

// FeatureExtractor transforms an incoming checkout payload into a feature set for ML model consumption.
type FeatureExtractor interface {
	// ExtractFeatures compiles the feature vector from payload and any historical telemetry.
	ExtractFeatures(ctx context.Context, payload Payload) (Features, error)
}

// CustomerStats contains historical customer return metrics.
type CustomerStats struct {
	CustomerID   string  `json:"customer_id"`
	TotalOrders  int     `json:"total_orders"`
	TotalReturns int     `json:"total_returns"`
	ReturnRate   float64 `json:"return_rate"`
}

// PincodeStats contains regional RTO risk factors.
type PincodeStats struct {
	Pincode string  `json:"pincode"`
	RTORate float64 `json:"rto_rate"`
}

// SKUStats contains catalog risk skew metrics.
type SKUStats struct {
	MaxSizeSkew float64 `json:"max_size_skew"`
}

// FeatureStore provides access to historical return telemetry for feature enrichment.
type FeatureStore interface {
	GetCustomerStats(ctx context.Context, customerID string) (CustomerStats, error)
	GetPincodeStats(ctx context.Context, pincode string) (PincodeStats, error)
	GetSKUStats(ctx context.Context, skus []string) (SKUStats, error)
}
