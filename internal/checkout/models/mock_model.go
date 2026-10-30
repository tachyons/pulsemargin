package models

import (
	"context"

	"pulsemargin/internal/checkout"
)

// MockMLModel enables tests to configure predictions or errors.
type MockMLModel struct {
	Prediction checkout.Prediction
	Err        error
}

func (m *MockMLModel) Name() string {
	return "mock_ml_model"
}

func (m *MockMLModel) Predict(ctx context.Context, features checkout.Features) (checkout.Prediction, error) {
	if m.Err != nil {
		return checkout.Prediction{}, m.Err
	}
	return m.Prediction, nil
}
