# PulseMargin

Return-risk scoring for e-commerce checkout, built on Exasol. The Go service takes a
cart, sends one analytical query to Exasol, and returns an interception tier. All the
feature aggregation and the logistic-regression scoring happen inside the database —
there is no separate model server.

## Setup

You need a running Exasol instance and Python 3 for the CSV generator. Install the Go
and Python versions with mise:

```bash
mise install
```

### Local Exasol (Exasol Personal)

If you don't already have a database, install the local Exasol Personal deployment:

```bash
exasol install local
exasol status          # want: database_ready
```

The local deployment ships with no script language containers, so the Python UDF cannot
be created until one is installed (this takes a few minutes):

```bash
exasol slc install PYTHON3
```

The local instance uses a self-signed certificate, so every `exasol connect` needs `-k`,
and the Go service connects with `validateservercertificate=0` (already set in
`internal/database/exasol.go`). See the "Connection troubleshooting" section if
`exasol` reports a stuck deployment state.

### Load the DDL

Load the schema and the scoring UDF first, either with `exasol connect` (local) or any
other SQL client (EXAplus, exapump, DBeaver):

```bash
exasol connect -k -f sql/schema.sql
exasol connect -k -f sql/udf_predict_return_risk.sql
```

Those two files are all the DDL there is. Verify with:

```bash
exasol connect -k -c "OPEN SCHEMA PULSE_MARGIN; SELECT COUNT(*) FROM pincode_risk_factors;"
```

### Load the data

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

## Connection troubleshooting

If the Go service logs `schema PULSE_MARGIN not found`, the DDL from "Load the DDL" has
not been applied yet.

If `exasol status` reports `operation_in_progress` or `deployment_failed` even though the
database port is open, the launcher state is stale — usually because a VM daemon was
orphaned by an interrupted command. Recover with:

```bash
# stop the orphaned daemon (find it with: ps aux | grep exasol-local-runner)
kill <pid>
# reset the recorded state to match the deployment, then restart
exasol start --auto-approve
exasol status          # want: database_ready
```

If the scoring UDF fails with `No usable script language container is installed`, the
Python SLC from "Local Exasol" is missing: `exasol slc install PYTHON3`.

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
