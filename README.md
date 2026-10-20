# PulseMargin

Return-risk scoring for e-commerce checkout, built on Exasol. The Go service takes a
cart, sends one analytical query to Exasol, and returns an interception tier. All the
feature aggregation and the logistic-regression scoring happen inside the database —
there is no separate model server.

## Setup

You need a running Exasol instance and Python 3.

Load the schema and the scoring UDF first, with any SQL client (EXAplus, exapump,
DBeaver):

```
sql/schema.sql
sql/udf_predict_return_risk.sql
```

Those two files are all the DDL there is.

Then generate and import the synthetic data. The seeder creates pincode and product
dimensions itself, and bulk-loads customers from the generated CSV:

```bash
make seed                 # 1M customer profiles
make seed SEED_ROWS=500000
```

Connection defaults are `localhost:8563`, user `sys`, password `exasol`, schema
`PULSE_MARGIN`. Override with `EXASOL_HOST`, `EXASOL_PORT`, `EXASOL_USER`,
`EXASOL_PASS`, `EXASOL_SCHEMA`. The API listens on `PORT` (default `8080`).

```bash
make run
```

## Scoring a checkout

```bash
curl -s localhost:8080/api/v1/checkout/score \
  -H 'Content-Type: application/json' \
  -d '{
    "checkout_id": "chk_101",
    "customer_id": "CUST_0000042",
    "pincode": "110001",
    "payment_mode": "COD",
    "items": [
      {"sku": "SKU-0001", "category": "Apparel_Jeans", "size": "M", "price": 1899},
      {"sku": "SKU-0001", "category": "Apparel_Jeans", "size": "L", "price": 1899}
    ]
  }'
```

```json
{
  "checkout_id": "chk_101",
  "risk_score": 0.83,
  "action_tier": "HIGH_RISK",
  "recommended_interventions": ["Trigger conversational WhatsApp sizing verification prior to fulfillment", "..."],
  "database_latency_ms": 7
}
```

The score is produced by `predict_return_risk`, a Python UDF that runs on the cluster.
Its inputs are the customer's historical return rate, the pincode RTO rate, the maximum
sizing skew across the cart, cart value, and whether the order is COD. Buying two sizes
of the same category counts as bracketing and raises the score.

`GET /healthz` is there for liveness checks.

## Layout

```
cmd/api       HTTP service
cmd/seed      data loader
internal/     config, Exasol connection, checkout scoring, HTTP handlers, seeding
sql/          schema and UDF
scripts/      CSV generator
```

`make test` runs the unit tests; they cover cart bracketing and tier boundaries, not the
database.
