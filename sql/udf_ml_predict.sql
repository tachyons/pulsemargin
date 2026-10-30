OPEN SCHEMA PULSE_MARGIN;

CREATE TABLE IF NOT EXISTS ml_model_registry (
    model_name    VARCHAR(64) PRIMARY KEY,
    model_type    VARCHAR(64),
    model_version VARCHAR(32),
    status        VARCHAR(32),
    parameters    VARCHAR(4000),
    updated_at    TIMESTAMP
);

--/
CREATE OR REPLACE PYTHON3 SCALAR SCRIPT predict_ml_risk(
    model_name VARCHAR(64),
    cust_return_rate DOUBLE,
    pincode_rto_rate DOUBLE,
    multi_size_detected BOOLEAN,
    size_skew DOUBLE,
    cart_value DOUBLE,
    is_cod BOOLEAN
) RETURNS VARCHAR(2000) AS
import json
import math

# In-database machine learning inference running on Exasol cluster
def run(ctx):
    c_rate = ctx.cust_return_rate if ctx.cust_return_rate is not None else 0.15
    p_rate = ctx.pincode_rto_rate if ctx.pincode_rto_rate is not None else 0.10
    skew   = ctx.size_skew if ctx.size_skew is not None else 0.0
    val    = ctx.cart_value if ctx.cart_value is not None else 0.0
    is_cod = bool(ctx.is_cod)
    bracket = bool(ctx.multi_size_detected)

    # Base log-odds prior
    log_odds = -2.10

    # Feature attributions
    f_cust = 3.65 * c_rate
    f_pin  = 2.30 * p_rate
    f_skew = 1.15 * skew
    f_val  = 0.00022 * val
    f_cod  = 1.10 if is_cod else 0.0
    f_bracket = 1.85 if bracket else 0.0

    # Non-linear feature interactions (compound risk)
    f_interaction = 0.0
    feature_importances = {}

    if bracket and is_cod and p_rate > 0.15:
        f_interaction += 0.85
        feature_importances["cod_bracket_pincode_interaction"] = 0.85

    if bracket and val > 3000:
        f_interaction += 0.45
        feature_importances["high_value_bracketing_interaction"] = 0.45

    log_odds += f_cust + f_pin + f_skew + f_val + f_cod + f_bracket + f_interaction
    prob = 1.0 / (1.0 + math.exp(-log_odds))
    prob = max(0.001, min(0.999, prob))

    if prob >= 0.80:
        tier = "HIGH_RISK"
    elif prob >= 0.50:
        tier = "MEDIUM_RISK"
    else:
        tier = "LOW_RISK"

    confidence = round(min(1.0, abs(prob - 0.5) * 2.0), 3)

    feature_importances["customer_return_rate"] = round(f_cust, 4)
    feature_importances["pincode_rto_rate"] = round(f_pin, 4)
    feature_importances["multi_size_detected"] = round(f_bracket, 4)
    feature_importances["size_skew"] = round(f_skew, 4)
    feature_importances["is_cod"] = round(f_cod, 4)
    feature_importances["cart_value"] = round(f_val, 4)

    result = {
        "risk_score": round(prob, 4),
        "predicted_tier": tier,
        "confidence": confidence,
        "model_version": ctx.model_name or "exasol_ml_v1",
        "feature_weights": feature_importances
    }

    return json.dumps(result)
/
