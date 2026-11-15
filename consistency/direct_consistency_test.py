"""Read-after-write test straight against the DynamoDB table (no API, no app).

Meant to run inside AWS (CloudShell in us-east-1), where a write followed by a
read takes ~1-2 ms. From a laptop every call adds a ~25 ms network round trip,
which gives DynamoDB's copies time to catch up, so stale reads are almost
never seen. Running next to the table gives eventual consistency a real chance
to show up.

Two tests, each repeated ROUNDS times:
  A. new item: PutItem a new cart, then GetItem it right away
     -> stale if the cart is not found yet
  B. update:   UpdateItem +1 on one cart, then GetItem it right away
     -> stale if the quantity is lower than what was just written

On a stale read, GetItem is retried until the data shows up, and the wait is
recorded (= how long until the read became consistent).

Usage: python3 direct_consistency_test.py <eventual|strong> [rounds]
Needs: boto3 (preinstalled in CloudShell), table shopping-carts in us-east-1.
"""

import json
import random
import sys
import time
from datetime import datetime, timezone

import boto3

TABLE = "shopping-carts"
REGION = "us-east-1"
RETRY_TIMEOUT = 2.0  # stop waiting for consistency after 2 s

db = boto3.client("dynamodb", region_name=REGION)


def now_utc():
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%fZ")


def new_cart_item(cart_id):
    # same item shape as the api writes; expires in 1 h so ttl cleans it up
    return {
        "cart_id": {"N": str(cart_id)},
        "customer_id": {"N": "9999"},
        "items": {"M": {}},
        "created_at": {"S": now_utc()},
        "expires_at": {"N": str(int(time.time()) + 3600)},
    }


def get_cart(cart_id, strong):
    out = db.get_item(TableName=TABLE, Key={"cart_id": {"N": str(cart_id)}}, ConsistentRead=strong)
    return out.get("Item")  # None if the read did not find the cart


def quantity(item):
    # quantity of product 1 inside the items map, 0 if missing
    if not item:
        return -1
    p = item["items"]["M"].get("1")
    return int(p["N"]) if p else 0


def wait_until(check):
    """Retries check() with no pause until True or RETRY_TIMEOUT. Returns (ok, retries, ms)."""
    start = time.perf_counter()
    retries = 0
    while time.perf_counter() - start < RETRY_TIMEOUT:
        retries += 1
        if check():
            return True, retries, round((time.perf_counter() - start) * 1000, 3)
    return False, retries, round((time.perf_counter() - start) * 1000, 3)


def test_new_item(rounds, strong):
    stale = []
    write_ms, read_ms = [], []
    for i in range(rounds):
        cart_id = random.randint(1, 2**31 - 1)
        t0 = time.perf_counter()
        db.put_item(TableName=TABLE, Item=new_cart_item(cart_id))
        t1 = time.perf_counter()
        item = get_cart(cart_id, strong)  # read right after the write returns
        t2 = time.perf_counter()
        write_ms.append((t1 - t0) * 1000)
        read_ms.append((t2 - t1) * 1000)
        if item is None:
            ok, retries, ms = wait_until(lambda: get_cart(cart_id, strong) is not None)
            stale.append({"round": i + 1, "cart_id": cart_id, "became_consistent": ok,
                          "retries": retries, "ms_until_consistent": ms, "timestamp": now_utc()})
    return summary(rounds, stale, write_ms, read_ms)


def test_update(rounds, strong):
    cart_id = random.randint(1, 2**31 - 1)
    db.put_item(TableName=TABLE, Item=new_cart_item(cart_id))
    wait_until(lambda: get_cart(cart_id, True) is not None)

    stale = []
    write_ms, read_ms = [], []
    for i in range(rounds):
        expected = i + 1
        t0 = time.perf_counter()
        # same update expression the api uses for "add item" (+1 of product 1)
        db.update_item(
            TableName=TABLE,
            Key={"cart_id": {"N": str(cart_id)}},
            UpdateExpression="SET #items.#p = if_not_exists(#items.#p, :zero) + :q",
            ExpressionAttributeNames={"#items": "items", "#p": "1"},
            ExpressionAttributeValues={":zero": {"N": "0"}, ":q": {"N": "1"}},
        )
        t1 = time.perf_counter()
        qty = quantity(get_cart(cart_id, strong))
        t2 = time.perf_counter()
        write_ms.append((t1 - t0) * 1000)
        read_ms.append((t2 - t1) * 1000)
        if qty < expected:
            ok, retries, ms = wait_until(lambda: quantity(get_cart(cart_id, strong)) >= expected)
            stale.append({"round": expected, "saw_quantity": qty, "became_consistent": ok,
                          "retries": retries, "ms_until_consistent": ms, "timestamp": now_utc()})
    return summary(rounds, stale, write_ms, read_ms)


def summary(rounds, stale, write_ms, read_ms):
    waits = [s["ms_until_consistent"] for s in stale if s["became_consistent"]]
    return {
        "rounds": rounds,
        "stale_reads": len(stale),
        "stale_percent": round(100 * len(stale) / rounds, 2),
        "max_ms_until_consistent": max(waits) if waits else 0,
        "avg_write_ms": round(sum(write_ms) / len(write_ms), 2),
        "avg_read_ms": round(sum(read_ms) / len(read_ms), 2),
        "stale_details": stale,
    }


def main():
    if len(sys.argv) < 2 or sys.argv[1] not in ("eventual", "strong"):
        print("Usage: python3 direct_consistency_test.py <eventual|strong> [rounds]")
        sys.exit(1)
    mode = sys.argv[1]
    strong = mode == "strong"
    rounds = int(sys.argv[2]) if len(sys.argv) > 2 else 500

    started = now_utc()
    a = test_new_item(rounds, strong)
    b = test_update(rounds, strong)
    results = {"mode": mode, "rounds_per_test": rounds, "started": started, "finished": now_utc(),
               "test_a_new_item": a, "test_b_update": b}

    out = f"direct-{mode}.json"
    with open(out, "w") as f:
        json.dump(results, f, indent=2)
    for name, r in (("A new item", a), ("B update", b)):
        print(f"{mode} | {name}: stale {r['stale_reads']}/{r['rounds']} ({r['stale_percent']}%), "
              f"max wait {r['max_ms_until_consistent']} ms, avg write {r['avg_write_ms']} ms, "
              f"avg read {r['avg_read_ms']} ms")
    print(f"started {started}, finished {results['finished']}, saved to {out}")


if __name__ == "__main__":
    main()
