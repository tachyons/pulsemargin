package checkout

import (
	"context"
	"strings"
)

// DefaultFeatureExtractor orchestrates cart analysis and telemetry retrieval from a FeatureStore.
type DefaultFeatureExtractor struct {
	store FeatureStore
}

// NewFeatureExtractor returns an extractor configured with the given FeatureStore.
func NewFeatureExtractor(store FeatureStore) *DefaultFeatureExtractor {
	return &DefaultFeatureExtractor{store: store}
}

// ExtractFeatures compiles the complete feature set required for machine learning model inference.
func (e *DefaultFeatureExtractor) ExtractFeatures(ctx context.Context, payload Payload) (Features, error) {
	cartTotal, hasBracketing := analyzeCart(payload.Items)

	categories := make([]string, 0, len(payload.Items))
	skus := make([]string, 0, len(payload.Items))
	catSeen := make(map[string]bool)

	for _, it := range payload.Items {
		skus = append(skus, it.SKU)
		if !catSeen[it.Category] {
			catSeen[it.Category] = true
			categories = append(categories, it.Category)
		}
	}

	custStats, err := e.store.GetCustomerStats(ctx, payload.CustomerID)
	if err != nil {
		custStats = CustomerStats{ReturnRate: 0.15}
	}

	pinStats, err := e.store.GetPincodeStats(ctx, payload.Pincode)
	if err != nil {
		pinStats = PincodeStats{RTORate: 0.10}
	}

	skuStats, err := e.store.GetSKUStats(ctx, skus)
	if err != nil {
		skuStats = SKUStats{MaxSizeSkew: 0.0}
	}

	isCOD := strings.EqualFold(payload.PaymentMode, "COD")

	return Features{
		CartTotal:          cartTotal,
		ItemCount:          len(payload.Items),
		HasBracketing:      hasBracketing,
		IsCOD:              isCOD,
		CustomerReturnRate: custStats.ReturnRate,
		PincodeRTORate:     pinStats.RTORate,
		MaxSizeSkew:        skuStats.MaxSizeSkew,
		Categories:         categories,
	}, nil
}
