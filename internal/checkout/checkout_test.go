package checkout

import (
	"strings"
	"testing"
)

func TestAnalyzeCart(t *testing.T) {
	tests := []struct {
		name        string
		items       []CartItem
		wantTotal   float64
		wantBracket bool
	}{
		{
			name:      "empty cart",
			items:     nil,
			wantTotal: 0,
		},
		{
			name: "single size per category is not bracketing",
			items: []CartItem{
				{Category: "Apparel_Jeans", Size: "M", Price: 1000},
				{Category: "Apparel_Shoes", Size: "UK8", Price: 2000},
			},
			wantTotal: 3000,
		},
		{
			name: "multiple sizes of one category is bracketing",
			items: []CartItem{
				{Category: "Apparel_Jeans", Size: "M", Price: 1000},
				{Category: "Apparel_Jeans", Size: "L", Price: 1000},
			},
			wantTotal:   2000,
			wantBracket: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total, bracketing := analyzeCart(tt.items)
			if total != tt.wantTotal {
				t.Errorf("total = %v, want %v", total, tt.wantTotal)
			}
			if bracketing != tt.wantBracket {
				t.Errorf("bracketing = %v, want %v", bracketing, tt.wantBracket)
			}
		})
	}
}

func TestNewDecisionTiers(t *testing.T) {
	tests := []struct {
		score float64
		tier  string
	}{
		{0.90, "HIGH_RISK"},
		{0.80, "HIGH_RISK"},
		{0.60, "MEDIUM_RISK"},
		{0.50, "MEDIUM_RISK"},
		{0.49, "LOW_RISK"},
		{0.00, "LOW_RISK"},
	}

	for _, tt := range tests {
		decision := newDecision("chk_1", tt.score, 1)
		if decision.ActionTier != tt.tier {
			t.Errorf("score %v: tier = %s, want %s", tt.score, decision.ActionTier, tt.tier)
		}
		if len(decision.Interventions) == 0 {
			t.Errorf("score %v: expected interventions", tt.score)
		}
	}
}

func TestScoreQueryArgsMatchPlaceholders(t *testing.T) {
	payload := Payload{
		CustomerID:  "CUST_1",
		Pincode:     "110001",
		PaymentMode: "COD",
		Items: []CartItem{
			{SKU: "SKU-0001"},
			{SKU: "SKU-0002"},
			{SKU: "SKU-0003"},
		},
	}

	query, args := scoreQuery(payload, 1500, true)

	if got, want := strings.Count(query, "?"), len(args); got != want {
		t.Fatalf("placeholders = %d, args = %d", got, want)
	}
	if args[len(args)-1] != "true" {
		t.Errorf("COD payment mode should produce a \"true\" final arg, got %v", args[len(args)-1])
	}
}
