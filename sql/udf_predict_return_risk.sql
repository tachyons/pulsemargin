OPEN SCHEMA PULSE_MARGIN;
--/
CREATE OR REPLACE PYTHON3 SCALAR SCRIPT predict_return_risk(
    cust_return_rate DOUBLE,
    pincode_rto_rate DOUBLE,
    multi_size_detected BOOLEAN,
    size_skew DOUBLE,
    cart_value DOUBLE,
    is_cod BOOLEAN
) RETURNS DOUBLE AS
import math

def run(ctx):
    # Coefficients trained from historical e-commerce returns
    w_intercept  = -2.15
    w_cust_rate  =  3.80
    w_pin_rate   =  2.40
    w_multi_size =  1.95  # Customer buying both 'M' and 'L' of the same item
    w_size_skew  =  1.20
    w_cod        =  1.10
    w_value_norm =  0.0002

    logit = (
        w_intercept +
        (w_cust_rate * (ctx.cust_return_rate or 0.15)) +
        (w_pin_rate * (ctx.pincode_rto_rate or 0.10)) +
        (w_multi_size if ctx.multi_size_detected else 0.0) +
        (w_size_skew * (ctx.size_skew or 0.0)) +
        (w_cod if ctx.is_cod else 0.0) +
        (w_value_norm * (ctx.cart_value or 0.0))
    )

    return float(1.0 / (1.0 + math.exp(-logit)))
/
