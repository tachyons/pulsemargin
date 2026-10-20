package seed

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
)

// Run seeds pincode risk factors, product telemetry and bulk-loads customer
// profiles from the generated CSV.
func Run(ctx context.Context, db *sql.DB, customerCSV string) error {
	if err := seedPincodes(ctx, db); err != nil {
		return err
	}
	if err := seedProducts(ctx, db); err != nil {
		return err
	}
	return importCustomerProfiles(ctx, db, customerCSV)
}

func seedPincodes(ctx context.Context, db *sql.DB) error {
	log.Println("Seeding pincode_risk_factors...")
	pincodes := []string{"110001", "400001", "560001", "600001", "700001", "670001", "500001", "380001"}

	stmt, err := db.PrepareContext(ctx, `
		MERGE INTO pincode_risk_factors p
		USING (SELECT ? AS pincode, ? AS rto_rate, ? AS courier_reliability, ? AS is_high_risk_zone FROM DUAL) src
		ON p.pincode = src.pincode
		WHEN MATCHED THEN UPDATE SET p.rto_rate = src.rto_rate, p.courier_reliability = src.courier_reliability
		WHEN NOT MATCHED THEN INSERT VALUES (src.pincode, src.rto_rate, src.courier_reliability, src.is_high_risk_zone);
	`)
	if err != nil {
		return fmt.Errorf("prepare pincode statement: %w", err)
	}
	defer stmt.Close()

	r := rand.New(rand.NewSource(42))
	for _, pin := range pincodes {
		rtoRate := 0.05 + r.Float64()*0.35
		reliability := 0.70 + r.Float64()*0.28
		isHighRisk := rtoRate > 0.25
		if _, err := stmt.ExecContext(ctx, pin, rtoRate, reliability, isHighRisk); err != nil {
			log.Printf("Error inserting pincode %s: %v", pin, err)
		}
	}

	return nil
}

func seedProducts(ctx context.Context, db *sql.DB) error {
	log.Println("Seeding product_return_telemetry...")
	categories := []string{"Apparel_Jeans", "Apparel_Shoes", "Electronics_Audio", "Apparel_Dresses"}
	sizes := []string{"S", "M", "L", "XL", "UK8", "UK9", "Standard"}

	stmt, err := db.PrepareContext(ctx, `
		MERGE INTO product_return_telemetry p
		USING (SELECT ? AS sku, ? AS category, ? AS size, ? AS size_skew_factor, ? AS return_rate FROM DUAL) src
		ON p.sku = src.sku
		WHEN MATCHED THEN UPDATE SET p.size_skew_factor = src.size_skew_factor
		WHEN NOT MATCHED THEN INSERT VALUES (src.sku, src.category, src.size, src.size_skew_factor, src.return_rate);
	`)
	if err != nil {
		return fmt.Errorf("prepare product statement: %w", err)
	}
	defer stmt.Close()

	r := rand.New(rand.NewSource(1337))
	for i := 1; i <= 500; i++ {
		sku := fmt.Sprintf("SKU-%04d", i)
		cat := categories[r.Intn(len(categories))]
		size := sizes[r.Intn(len(sizes))]
		sizeSkew := r.Float64() * 0.90
		returnRate := 0.10 + r.Float64()*0.30

		if _, err := stmt.ExecContext(ctx, sku, cat, size, sizeSkew, returnRate); err != nil {
			log.Printf("Error inserting SKU %s: %v", sku, err)
		}
	}

	return nil
}

// importCustomerProfiles streams a local CSV through the driver's IMPORT path,
// which keeps the load parallel across the Exasol cluster.
func importCustomerProfiles(ctx context.Context, db *sql.DB, csvPath string) error {
	absPath, err := filepath.Abs(csvPath)
	if err != nil {
		return fmt.Errorf("resolve csv path: %w", err)
	}
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("customer csv %s: %w", absPath, err)
	}

	log.Printf("Importing customer profiles from %s...", absPath)

	if _, err := db.ExecContext(ctx, "TRUNCATE TABLE customer_profiles"); err != nil {
		return fmt.Errorf("truncate customer_profiles: %w", err)
	}

	query := fmt.Sprintf(
		"IMPORT INTO customer_profiles FROM LOCAL CSV FILE '%s' COLUMN SEPARATOR = ',' ROW SEPARATOR = 'LF'",
		strings.ReplaceAll(absPath, "'", "''"),
	)
	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("import customer_profiles: %w", err)
	}

	log.Println("Customer profiles imported.")
	return nil
}
