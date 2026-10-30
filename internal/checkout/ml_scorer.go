package checkout

import (
	"context"
	"fmt"
	"time"
)

// MLScorer coordinates feature extraction, ML model inference, and categorizer evaluation.
type MLScorer struct {
	extractor   FeatureExtractor
	model       MLModel
	categorizer Categorizer
}

// NewMLScorer creates an instance of MLScorer with the provided interface implementations.
func NewMLScorer(extractor FeatureExtractor, model MLModel, categorizer Categorizer) *MLScorer {
	if categorizer == nil {
		categorizer = NewMLCategorizer()
	}
	return &MLScorer{
		extractor:   extractor,
		model:       model,
		categorizer: categorizer,
	}
}

// Score executes the end-to-end machine learning scoring pipeline.
func (s *MLScorer) Score(ctx context.Context, payload Payload) (Decision, error) {
	start := time.Now()

	features, err := s.extractor.ExtractFeatures(ctx, payload)
	if err != nil {
		return Decision{}, fmt.Errorf("feature extraction failed for checkout %s: %w", payload.CheckoutID, err)
	}

	prediction, err := s.model.Predict(ctx, features)
	if err != nil {
		return Decision{}, fmt.Errorf("model prediction failed for checkout %s: %w", payload.CheckoutID, err)
	}

	tier, interventions, err := s.categorizer.Categorize(ctx, payload, prediction)
	if err != nil {
		return Decision{}, fmt.Errorf("categorization failed for checkout %s: %w", payload.CheckoutID, err)
	}

	return Decision{
		CheckoutID:    payload.CheckoutID,
		RiskScore:     prediction.RiskScore,
		ActionTier:    tier,
		Interventions: interventions,
		LatencyMs:     time.Since(start).Milliseconds(),
		ModelVersion:  prediction.ModelVersion,
	}, nil
}
