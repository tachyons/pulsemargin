#set page(width: 338.7mm, height: 190.5mm, margin: 0mm)
#set text(font: "Helvetica", size: 20pt)
#show heading.where(level: 1): it => block(below: 1em)[
  #set text(size: 34pt, weight: "bold")
  #it
]
#show heading.where(level: 2): it => block(below: 0.6em)[
  #set text(size: 22pt, fill: rgb("#0b6"))
  #it
]

#let accent = rgb("#0b6")
#let dim = rgb("#555")

#let slide(body) = page(margin: 2cm)[#body]

// Slide 1 — Title
#slide[
  #v(1fr)
  #align(center)[
    #text(size: 58pt, weight: "bold")[PulseMargin]
    #v(0.4em)
    #text(size: 26pt, fill: dim)[Return-risk scoring at checkout, inside Exasol]
    #v(2em)
    #text(size: 18pt)[ExaThon India 2026 · Aboobacker MK · Sijin TV]
  ]
  #v(1fr)
]

// Slide 2 — Problem
#slide[
  = The problem
  #v(0.5em)
  Indian e-commerce bleeds margin to returns:
  #v(0.4em)
  - *COD refusals* — the parcel is rejected at the door. Seller pays two-way shipping.
  - *Size bracketing* — "order M and L, return one." Apparel return rates exceed 30% in
    high-RTO pincodes.
  #v(0.6em)
  Today this is caught after fulfilment, when the only options are a refund and a loss.
]

// Slide 3 — Insight
#slide[
  = The decision is at checkout
  #v(0.5em)
  Is *this cart*, from *this customer*, to *this pincode*, on *this payment mode*, likely
  to come back?
  #v(0.6em)
  Answer it in one query, before the parcel ships. Then intervene:
  #v(0.4em)
  - Verify sizing before dispatch
  - Convert COD to prepaid
  - Route to a cheaper courier tier
]

// Slide 4 — Solution
#slide[
  = What we built
  #v(0.5em)
  A Go service that takes a cart and returns an interception tier, scored entirely
  *in-database*.
  #v(0.6em)
  - No model server. No Python microservice. No feature store to keep in sync.
  - Feature aggregation + logistic regression run as a `PYTHON3` UDF inside Exasol.
  - One round trip. Latency is a property of the warehouse.
]

// Slide 5 — How it works
#slide[
  = How it works
  #v(0.5em)
  1. Checkout service POSTs the cart to `/api/v1/checkout/score`
  2. One CTE query joins: customer history · pincode RTO · SKU sizing skew
  3. `predict_return_risk` UDF scores: customer return rate, pincode RTO, size bracketing,
     size skew, cart value, COD
  4. Result maps to `LOW` / `MEDIUM` / `HIGH_RISK` with tailored interventions
]

// Slide 6 — Why Exasol
#slide[
  = Why Exasol
  #v(0.5em)
  - *In-database scoring* — the join across 1M customer profiles, pincode and SKU
    dimensions is what the warehouse is built for.
  - *Elastic, parallel UDFs* — Python runs alongside the data, not over the network.
  - *Single source of truth* — risk features and the model move together with the data.
]

// Slide 7 — Demo
#slide[
  = Demo
  #v(0.5em)
  ```bash
  curl -s localhost:8080/api/v1/checkout/score \
    -H 'Content-Type: application/json' \
    -d '{"checkout_id":"chk_101","customer_id":"CUST_0000042",
         "pincode":"110001","payment_mode":"COD",
         "items":[{"sku":"SKU-0001","category":"Apparel_Jeans","size":"M","price":1899},
                  {"sku":"SKU-0001","category":"Apparel_Jeans","size":"L","price":1899}]}'
  ```
  #v(0.4em)
  #text(fill: accent, weight: "bold")[→ risk_score 0.95 · HIGH_RISK · 102 ms in Exasol]
  Two sizes of one SKU (bracketing) + COD + a high-RTO pincode floors the cart.
]

// Slide 8 — Status
#slide[
  = Status
  #v(0.5em)
  - Working end to end on Exasol Personal: schema, UDF, 1M-row seed, live API, web UI.
  - Go · `go test` green · CI on GitHub Actions with the toolchain pinned via `mise`.
  - MIT licensed. Minimal Oat UI embedded in the binary.
  #v(1.2em)
  #text(size: 30pt, weight: "bold", fill: accent)[github.com/tachyons/pulsemargin]
]
