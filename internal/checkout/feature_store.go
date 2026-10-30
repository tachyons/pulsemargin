package checkout

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
)

// InMemoryFeatureStore provides in-memory telemetry lookups for offline or demo scoring.
type InMemoryFeatureStore struct {
	mu           sync.RWMutex
	customers    map[string]CustomerStats
	pincodes     map[string]PincodeStats
	skuSkew      map[string]float64
	defaultCust  CustomerStats
	defaultPin   PincodeStats
}

// NewInMemoryFeatureStore initializes an in-memory feature store with sensible baseline defaults.
func NewInMemoryFeatureStore() *InMemoryFeatureStore {
	store := &InMemoryFeatureStore{
		customers: make(map[string]CustomerStats),
		pincodes:  make(map[string]PincodeStats),
		skuSkew:   make(map[string]float64),
		defaultCust: CustomerStats{
			ReturnRate: 0.15,
		},
		defaultPin: PincodeStats{
			RTORate: 0.10,
		},
	}

	// Seed with representative risk profiles
	store.pincodes["110001"] = PincodeStats{Pincode: "110001", RTORate: 0.12}
	store.pincodes["400001"] = PincodeStats{Pincode: "400001", RTORate: 0.08}
	store.pincodes["560001"] = PincodeStats{Pincode: "560001", RTORate: 0.07}
	store.pincodes["700001"] = PincodeStats{Pincode: "700001", RTORate: 0.18}
	store.pincodes["800001"] = PincodeStats{Pincode: "800001", RTORate: 0.28} // High RTO tier

	store.customers["CUST_HIGH_RISK"] = CustomerStats{
		CustomerID:   "CUST_HIGH_RISK",
		TotalOrders:  20,
		TotalReturns: 14,
		ReturnRate:   0.70,
	}
	store.customers["CUST_LOW_RISK"] = CustomerStats{
		CustomerID:   "CUST_LOW_RISK",
		TotalOrders:  15,
		TotalReturns: 1,
		ReturnRate:   0.067,
	}

	store.skuSkew["SKU-HIGH-SKEW"] = 0.85
	store.skuSkew["SKU-MED-SKEW"] = 0.40

	return store
}

func (s *InMemoryFeatureStore) SetCustomerStats(stats CustomerStats) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.customers[stats.CustomerID] = stats
}

func (s *InMemoryFeatureStore) SetPincodeStats(stats PincodeStats) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pincodes[stats.Pincode] = stats
}

func (s *InMemoryFeatureStore) SetSKUSkew(sku string, skew float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.skuSkew[sku] = skew
}

func (s *InMemoryFeatureStore) GetCustomerStats(ctx context.Context, customerID string) (CustomerStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if stats, found := s.customers[customerID]; found {
		return stats, nil
	}
	return CustomerStats{CustomerID: customerID, ReturnRate: s.defaultCust.ReturnRate}, nil
}

func (s *InMemoryFeatureStore) GetPincodeStats(ctx context.Context, pincode string) (PincodeStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if stats, found := s.pincodes[pincode]; found {
		return stats, nil
	}
	return PincodeStats{Pincode: pincode, RTORate: s.defaultPin.RTORate}, nil
}

func (s *InMemoryFeatureStore) GetSKUStats(ctx context.Context, skus []string) (SKUStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var maxSkew float64
	for _, sku := range skus {
		if skew, found := s.skuSkew[sku]; found && skew > maxSkew {
			maxSkew = skew
		}
	}
	return SKUStats{MaxSizeSkew: maxSkew}, nil
}

// DatabaseFeatureStore queries telemetry tables in Exasol to construct features.
type DatabaseFeatureStore struct {
	db *sql.DB
}

// NewDatabaseFeatureStore creates a FeatureStore backed by the database.
func NewDatabaseFeatureStore(db *sql.DB) *DatabaseFeatureStore {
	return &DatabaseFeatureStore{db: db}
}

func (d *DatabaseFeatureStore) GetCustomerStats(ctx context.Context, customerID string) (CustomerStats, error) {
	query := `
		SELECT
			COALESCE(total_orders, 0),
			COALESCE(total_returns, 0),
			CASE WHEN total_orders > 0 THEN CAST(total_returns AS DOUBLE) / total_orders ELSE 0.15 END
		FROM customer_profiles
		WHERE customer_id = ?;
	`
	var orders, returns int
	var retRate float64
	err := d.db.QueryRowContext(ctx, query, customerID).Scan(&orders, &returns, &retRate)
	if err != nil {
		if err == sql.ErrNoRows {
			return CustomerStats{CustomerID: customerID, ReturnRate: 0.15}, nil
		}
		return CustomerStats{}, fmt.Errorf("fetch customer stats for %s: %w", customerID, err)
	}

	return CustomerStats{
		CustomerID:   customerID,
		TotalOrders:  orders,
		TotalReturns: returns,
		ReturnRate:   retRate,
	}, nil
}

func (d *DatabaseFeatureStore) GetPincodeStats(ctx context.Context, pincode string) (PincodeStats, error) {
	query := `
		SELECT COALESCE(rto_rate, 0.10)
		FROM pincode_risk_factors
		WHERE pincode = ?;
	`
	var rtoRate float64
	err := d.db.QueryRowContext(ctx, query, pincode).Scan(&rtoRate)
	if err != nil {
		if err == sql.ErrNoRows {
			return PincodeStats{Pincode: pincode, RTORate: 0.10}, nil
		}
		return PincodeStats{}, fmt.Errorf("fetch pincode stats for %s: %w", pincode, err)
	}

	return PincodeStats{
		Pincode: pincode,
		RTORate: rtoRate,
	}, nil
}

func (d *DatabaseFeatureStore) GetSKUStats(ctx context.Context, skus []string) (SKUStats, error) {
	if len(skus) == 0 {
		return SKUStats{MaxSizeSkew: 0.0}, nil
	}

	placeholders := make([]string, len(skus))
	args := make([]any, len(skus))
	for i, sku := range skus {
		placeholders[i] = "?"
		args[i] = sku
	}

	query := fmt.Sprintf(`
		SELECT COALESCE(MAX(size_skew_factor), 0.0)
		FROM product_return_telemetry
		WHERE sku IN (%s);
	`, strings.Join(placeholders, ","))

	var maxSkew float64
	err := d.db.QueryRowContext(ctx, query, args...).Scan(&maxSkew)
	if err != nil {
		if err == sql.ErrNoRows {
			return SKUStats{MaxSizeSkew: 0.0}, nil
		}
		return SKUStats{}, fmt.Errorf("fetch sku stats: %w", err)
	}

	return SKUStats{MaxSizeSkew: maxSkew}, nil
}
