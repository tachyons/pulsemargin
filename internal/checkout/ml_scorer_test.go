package checkout

import (
	"context"
	"errors"
	"testing"
)

type stubModel struct {
	prediction Prediction
	err        error
}

func (m *stubModel) Name() string { return "stub" }
func (m *stubModel) Predict(ctx context.Context, features Features) (Prediction, error) {
	if m.err != nil {
		return Prediction{}, m.err
	}
	return m.prediction, nil
}

func TestMLScorer(t *testing.T) {
	store := NewInMemoryFeatureStore()
	store.SetCustomerStats(CustomerStats{
		CustomerID:  "CUST_TEST",
		TotalOrders: 10,
		ReturnRate:  0.40,
	})
	store.SetPincodeStats(PincodeStats{
		Pincode: "110001",
		RTORate: 0.15,
	})

	extractor := NewFeatureExtractor(store)

	t.Run("successful end to end scoring", func(t *testing.T) {
		model := &stubModel{
			prediction: Prediction{
				RiskScore:     0.82,
				PredictedTier: "HIGH_RISK",
				Confidence:    0.90,
				ModelVersion:  "stub_v1",
				FeatureWeights: map[string]float64{
					"multi_size_detected": 1.95,
				},
			},
		}

		scorer := NewMLScorer(extractor, model, NewMLCategorizer())
		payload := Payload{
			CheckoutID:  "chk_101",
			CustomerID:  "CUST_TEST",
			Pincode:     "110001",
			PaymentMode: "COD",
			Items: []CartItem{
				{SKU: "SKU-1", Category: "Jeans", Size: "M", Price: 1000},
				{SKU: "SKU-1", Category: "Jeans", Size: "L", Price: 1000},
			},
		}

		decision, err := scorer.Score(context.Background(), payload)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if decision.CheckoutID != "chk_101" {
			t.Errorf("got checkout ID %s, want chk_101", decision.CheckoutID)
		}
		if decision.RiskScore != 0.82 {
			t.Errorf("got risk score %f, want 0.82", decision.RiskScore)
		}
		if decision.ActionTier != "HIGH_RISK" {
			t.Errorf("got tier %s, want HIGH_RISK", decision.ActionTier)
		}
		if decision.ModelVersion != "stub_v1" {
			t.Errorf("got model version %s, want stub_v1", decision.ModelVersion)
		}
		if len(decision.Interventions) == 0 {
			t.Errorf("expected interventions")
		}
	})

	t.Run("model inference failure bubbles up error", func(t *testing.T) {
		model := &stubModel{
			err: errors.New("inference timeout"),
		}
		scorer := NewMLScorer(extractor, model, NewMLCategorizer())

		_, err := scorer.Score(context.Background(), Payload{CheckoutID: "chk_fail"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestInMemoryFeatureStore(t *testing.T) {
	store := NewInMemoryFeatureStore()
	ctx := context.Background()

	// Default customer return rate
	cust, err := store.GetCustomerStats(ctx, "UNKNOWN_CUST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cust.ReturnRate != 0.15 {
		t.Errorf("expected default return rate 0.15, got %f", cust.ReturnRate)
	}

	// Known customer
	store.SetCustomerStats(CustomerStats{
		CustomerID: "CUST_KNOWN",
		ReturnRate: 0.55,
	})
	cust, err = store.GetCustomerStats(ctx, "CUST_KNOWN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cust.ReturnRate != 0.55 {
		t.Errorf("expected return rate 0.55, got %f", cust.ReturnRate)
	}

	// SKU stats
	store.SetSKUSkew("SKU_A", 0.75)
	skuStats, err := store.GetSKUStats(ctx, []string{"SKU_A", "SKU_B"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skuStats.MaxSizeSkew != 0.75 {
		t.Errorf("expected max skew 0.75, got %f", skuStats.MaxSizeSkew)
	}
}
