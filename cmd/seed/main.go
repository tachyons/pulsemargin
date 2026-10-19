package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/exasol/exasol-driver-go"
)

func main() {
	host := getEnv("EXASOL_HOST", "localhost")
	port := getEnv("EXASOL_PORT", "8563")
	user := getEnv("EXASOL_USER", "sys")
	pass := getEnv("EXASOL_PASS", "exasol")
	schema := getEnv("EXASOL_SCHEMA", "PULSE_MARGIN")

	dsn := fmt.Sprintf("exa:%s:%s;user=%s;password=%s;schema=%s", host, port, user, pass, schema)
	db, err := sql.Open("exasol", dsn)
	if err != nil {
		log.Fatalf("Connection failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	log.Println("Starting Exasol synthetic data pipeline...")

	seedPincodes(ctx, db)
	seedProducts(ctx, db)
	seedCustomerProfiles(ctx, db, 100_000) // Adjust volume as needed for scale demo

	log.Println("Data seeding completed successfully.")
}

func seedPincodes(ctx context.Context, db *sql.DB) {
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
		log.Fatalf("Failed to prepare pincode statement: %v", err)
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
}

func seedProducts(ctx context.Context, db *sql.DB) {
	log.Println("Seeding product_return_telemetry...")
	categories := []string{"Apparel_Jeans", "Apparel_Shoes", "Electronics_Audio", "Apparel_Dresses"}
	sizes := []string{"S", "M", "L", "XL", "UK8", "UK9", "Standard"}

	r := rand.New(rand.NewSource(1337))

	for i := 1; i <= 500; i++ {
		sku := fmt.Sprintf("SKU-%04d", i)
		cat := categories[r.Intn(len(categories))]
		size := sizes[r.Intn(len(sizes))]
		sizeSkew := r.Float64() * 0.90
		returnRate := 0.10 + r.Float64()*0.30

		query := `
			MERGE INTO product_return_telemetry p
			USING (SELECT ? AS sku, ? AS category, ? AS size, ? AS size_skew_factor, ? AS return_rate FROM DUAL) src
			ON p.sku = src.sku
			WHEN MATCHED THEN UPDATE SET p.size_skew_factor = src.size_skew_factor
			WHEN NOT MATCHED THEN INSERT VALUES (src.sku, src.category, src.size, src.size_skew_factor, src.return_rate);
		`
		if _, err := db.ExecContext(ctx, query, sku, cat, size, sizeSkew, returnRate); err != nil {
			log.Printf("Error inserting SKU %s: %v", sku, err)
		}
	}
}

func seedCustomerProfiles(ctx context.Context, db *sql.DB, totalCount int) {
	log.Printf("Seeding %d customer profiles in batches...", totalCount)
	batchSize := 5000
	workers := 4
	jobs := totalCount / batchSize

	var wg sync.WaitGroup
	jobChan := make(chan int, jobs)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))

			for batchIdx := range jobChan {
				startCustID := batchIdx * batchSize
				valStrings := make([]string, 0, batchSize)
				valArgs := make([]interface{}, 0, batchSize*6)

				for i := 0; i < batchSize; i++ {
					cID := fmt.Sprintf("CUST_%07d", startCustID+i)
					orders := r.Intn(35) + 1
					returns := int(float64(orders) * (r.Float64() * 0.50))
					rto := int(float64(returns) * (r.Float64() * 0.40))
					aov := 500.0 + r.Float64()*4500.0
					dateStr := time.Now().AddDate(0, -r.Intn(24), -r.Intn(30)).Format("2006-01-02 15:04:05")

					valStrings = append(valStrings, "(?, ?, ?, ?, ?, ?)")
					valArgs = append(valArgs, cID, orders, returns, rto, aov, dateStr)
				}

				query := fmt.Sprintf(
					"INSERT INTO customer_profiles (customer_id, total_orders, total_returns, rto_count, avg_order_value, account_created_at) VALUES %s;",
					strings.Join(valStrings, ","),
				)

				if _, err := db.ExecContext(ctx, query, valArgs...); err != nil {
					log.Printf("Worker %d batch %d failed: %v", workerID, batchIdx, err)
				} else {
					log.Printf("Worker %d inserted batch %d/%d (%d rows)", workerID, batchIdx+1, jobs, batchSize)
				}
			}
		}(w)
	}

	for j := 0; j < jobs; j++ {
		jobChan <- j
	}
	close(jobChan)

	wg.Wait()
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
