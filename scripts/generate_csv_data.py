#!/usr/bin/env python3
"""Generate synthetic customer_profiles rows for bulk IMPORT into Exasol."""

import argparse
import csv
import os
import random
from datetime import datetime, timedelta


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--rows", type=int, default=1_000_000)
    parser.add_argument("--out", default="data/customer_profiles.csv")
    args = parser.parse_args()

    os.makedirs(os.path.dirname(args.out) or ".", exist_ok=True)
    random.seed(42)
    now = datetime.now()

    with open(args.out, "w", newline="") as f:
        writer = csv.writer(f, lineterminator="\n")
        for i in range(args.rows):
            orders = random.randint(1, 40)
            returns = int(orders * random.uniform(0.05, 0.45))
            rto = int(returns * random.uniform(0.0, 0.5))
            aov = round(random.uniform(500, 8000), 2)
            created = now - timedelta(
                days=random.randint(0, 730), minutes=random.randint(0, 1440)
            )
            writer.writerow([
                f"CUST_{i:07d}",
                orders,
                returns,
                rto,
                aov,
                created.strftime("%Y-%m-%d %H:%M:%S"),
            ])

    print(f"Wrote {args.rows} rows to {args.out}")


if __name__ == "__main__":
    main()
