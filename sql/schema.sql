CREATE SCHEMA IF NOT EXISTS PULSE_MARGIN;
OPEN SCHEMA PULSE_MARGIN;

-- Historical customer baseline
CREATE TABLE IF NOT EXISTS customer_profiles (
    customer_id         VARCHAR(64) PRIMARY KEY,
    total_orders        INTEGER,
    total_returns       INTEGER,
    rto_count           INTEGER,
    avg_order_value     DECIMAL(10,2),
    account_created_at  TIMESTAMP
);

-- Delivery risk by postal code (logistics performance & historical fraud/RTO)
CREATE TABLE IF NOT EXISTS pincode_risk_factors (
    pincode             VARCHAR(10) PRIMARY KEY,
    rto_rate            DOUBLE PRECISION,
    courier_reliability DOUBLE PRECISION,
    is_high_risk_zone   BOOLEAN
);

-- Sizing return friction metrics per SKU/Category
CREATE TABLE IF NOT EXISTS product_return_telemetry (
    sku                 VARCHAR(64) PRIMARY KEY,
    category            VARCHAR(64),
    size                VARCHAR(10),
    size_skew_factor    DOUBLE PRECISION,
    return_rate         DOUBLE PRECISION
);
