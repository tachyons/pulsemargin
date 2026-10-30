package models

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"pulsemargin/internal/checkout"
)

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestLogisticRegressionModel(t *testing.T) {
	model := NewLogisticRegressionModel("logistic_v1", DefaultLogisticWeights())

	t.Run("high risk cart with bracketing and COD", func(t *testing.T) {
		features := checkout.Features{
			CartTotal:          4000,
			HasBracketing:      true,
			IsCOD:              true,
			CustomerReturnRate: 0.65,
			PincodeRTORate:     0.25,
			MaxSizeSkew:        0.75,
		}

		pred, err := model.Predict(context.Background(), features)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if pred.RiskScore < 0.80 {
			t.Errorf("expected high risk score >= 0.80, got %f", pred.RiskScore)
		}
		if pred.PredictedTier != "HIGH_RISK" {
			t.Errorf("expected HIGH_RISK, got %s", pred.PredictedTier)
		}
		if pred.Confidence <= 0.0 {
			t.Errorf("expected positive confidence, got %f", pred.Confidence)
		}
		if pred.FeatureWeights["multi_size_detected"] <= 0 {
			t.Errorf("expected positive bracketing weight contribution")
		}
	})

	t.Run("low risk prepaid cart without bracketing", func(t *testing.T) {
		features := checkout.Features{
			CartTotal:          500,
			HasBracketing:      false,
			IsCOD:              false,
			CustomerReturnRate: 0.05,
			PincodeRTORate:     0.05,
			MaxSizeSkew:        0.0,
		}

		pred, err := model.Predict(context.Background(), features)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if pred.RiskScore >= 0.50 {
			t.Errorf("expected low risk score < 0.50, got %f", pred.RiskScore)
		}
		if pred.PredictedTier != "LOW_RISK" {
			t.Errorf("expected LOW_RISK, got %s", pred.PredictedTier)
		}
	})
}

func TestDecisionTreeModel(t *testing.T) {
	model := NewDecisionTreeModel("tree_v2")

	tests := []struct {
		name     string
		features checkout.Features
		wantTier string
	}{
		{
			name: "high customer return rate triggers high risk",
			features: checkout.Features{
				CustomerReturnRate: 0.50,
			},
			wantTier: "HIGH_RISK",
		},
		{
			name: "bracketing + COD + high pincode RTO triggers high risk",
			features: checkout.Features{
				HasBracketing:  true,
				IsCOD:          true,
				PincodeRTORate: 0.22,
			},
			wantTier: "HIGH_RISK",
		},
		{
			name: "bracketing alone triggers medium risk",
			features: checkout.Features{
				HasBracketing: true,
			},
			wantTier: "MEDIUM_RISK",
		},
		{
			name: "clean order triggers low risk",
			features: checkout.Features{
				CustomerReturnRate: 0.10,
				PincodeRTORate:     0.08,
				HasBracketing:      false,
			},
			wantTier: "LOW_RISK",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pred, err := model.Predict(context.Background(), tt.features)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pred.PredictedTier != tt.wantTier {
				t.Errorf("predicted tier = %s, want %s (score = %f)", pred.PredictedTier, tt.wantTier, pred.RiskScore)
			}
		})
	}
}

func TestRemoteModelClient(t *testing.T) {
	expectedPred := checkout.Prediction{
		RiskScore:     0.88,
		PredictedTier: "HIGH_RISK",
		Confidence:    0.92,
		ModelVersion:  "remote_v1",
	}

	t.Run("successful remote call with mock round tripper", func(t *testing.T) {
		mockClient := &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				bodyBytes, _ := json.Marshal(expectedPred)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
				}, nil
			}),
		}

		client := NewRemoteModelClient("http://models.internal/predict", 1*time.Second, nil)
		client.SetHTTPClient(mockClient)

		pred, err := client.Predict(context.Background(), checkout.Features{CartTotal: 1000})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pred.RiskScore != expectedPred.RiskScore {
			t.Errorf("got risk score %f, want %f", pred.RiskScore, expectedPred.RiskScore)
		}
		if pred.PredictedTier != expectedPred.PredictedTier {
			t.Errorf("got tier %s, want %s", pred.PredictedTier, expectedPred.PredictedTier)
		}
	})

	t.Run("fallback when remote unreachable", func(t *testing.T) {
		mockClient := &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("connection refused")
			}),
		}

		fallback := NewLogisticRegressionModel("fallback_v1", DefaultLogisticWeights())
		client := NewRemoteModelClient("http://models.internal/predict", 50*time.Millisecond, fallback)
		client.SetHTTPClient(mockClient)

		pred, err := client.Predict(context.Background(), checkout.Features{
			HasBracketing:      true,
			CustomerReturnRate: 0.60,
		})
		if err != nil {
			t.Fatalf("fallback should have succeeded, got: %v", err)
		}
		if pred.ModelVersion != "fallback_v1" {
			t.Errorf("expected fallback model version, got %s", pred.ModelVersion)
		}
	})
}

func TestExasolMLModel(t *testing.T) {
	fallback := NewLogisticRegressionModel("fallback_v1", DefaultLogisticWeights())
	model := NewExasolMLModel(nil, "exasol_ensemble_v2", fallback)

	if model.Name() != "exasol_ensemble_v2" {
		t.Errorf("got model name %s, want exasol_ensemble_v2", model.Name())
	}

	// Nil DB triggers fallback
	pred, err := model.Predict(context.Background(), checkout.Features{
		CartTotal:          3000,
		HasBracketing:      true,
		CustomerReturnRate: 0.50,
	})
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}

	if pred.ModelVersion != "fallback_v1" {
		t.Errorf("expected fallback model version, got %s", pred.ModelVersion)
	}
}
