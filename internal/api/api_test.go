package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"pulsemargin/internal/checkout"
)

type mockScorer struct {
	decision checkout.Decision
	err      error
}

func (m *mockScorer) Score(ctx context.Context, payload checkout.Payload) (checkout.Decision, error) {
	if m.err != nil {
		return checkout.Decision{}, m.err
	}
	return m.decision, nil
}

func TestAPIHealthz(t *testing.T) {
	handler := NewHandler(&mockScorer{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "UP" {
		t.Errorf("status = %s, want UP", resp["status"])
	}
}

func TestAPIScoreCheckout(t *testing.T) {
	expectedDecision := checkout.Decision{
		CheckoutID:    "chk_mock_1",
		RiskScore:     0.85,
		ActionTier:    "HIGH_RISK",
		Interventions: []string{"Trigger conversational WhatsApp sizing verification"},
		LatencyMs:     5,
		ModelVersion:  "mock_v1",
	}

	scorer := &mockScorer{decision: expectedDecision}
	handler := NewHandler(scorer)

	t.Run("valid payload succeeds", func(t *testing.T) {
		payload := checkout.Payload{
			CheckoutID:  "chk_mock_1",
			CustomerID:  "CUST_1",
			Pincode:     "110001",
			PaymentMode: "COD",
			Items: []checkout.CartItem{
				{SKU: "SKU-1", Category: "Shoes", Size: "9", Price: 2500},
			},
		}

		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/checkout/score", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d (body: %s)", rec.Code, rec.Body.String())
		}

		var dec checkout.Decision
		if err := json.Unmarshal(rec.Body.Bytes(), &dec); err != nil {
			t.Fatalf("failed to parse decision: %v", err)
		}

		if dec.RiskScore != expectedDecision.RiskScore {
			t.Errorf("score = %f, want %f", dec.RiskScore, expectedDecision.RiskScore)
		}
		if dec.ActionTier != expectedDecision.ActionTier {
			t.Errorf("tier = %s, want %s", dec.ActionTier, expectedDecision.ActionTier)
		}
	})

	t.Run("missing required fields returns 400", func(t *testing.T) {
		payload := checkout.Payload{
			CheckoutID: "chk_incomplete",
			// Missing customer_id and items
		}

		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/checkout/score", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", rec.Code)
		}
	})

	t.Run("scoring failure returns 500", func(t *testing.T) {
		failingScorer := &mockScorer{err: errors.New("underlying model error")}
		failingHandler := NewHandler(failingScorer)

		payload := checkout.Payload{
			CheckoutID: "chk_err",
			CustomerID: "CUST_1",
			Items:      []checkout.CartItem{{SKU: "SKU-1"}},
		}

		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/checkout/score", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		failingHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 500 Internal Server Error, got %d", rec.Code)
		}
	})
}
