"""Read-after-write consistency tests for the shopping cart API.

Runs the 3 scenarios from Step II Part 3 against any deployment of the API
(MySQL or DynamoDB), so the two can be compared:

  1. create a cart, then GET it right away         -> is the new cart visible?
  2. add an item, then GET the cart right away      -> is the new quantity visible?
  3. many clients add to the same cart at once      -> are any updates lost?

A "stale read" is a GET that returns older data than what was just written
(404 for a cart that exists, or a quantity lower than expected). When one
happens, the GET is retried until the data shows up, and the wait is recorded
as the time until the read became consistent.

Usage: python consistency_test.py <server_url> <label> <output_file>
  e.g. python consistency_test.py http://1.2.3.4:8080 dynamo-eventual results.json
"""

import json
import sys
import threading
import time
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone

import requests

ROUNDS = 50            # rounds for tests 1 and 2
CLIENTS = 10           # concurrent clients in test 3
ADDS_PER_CLIENT = 10   # adds per client in test 3
RETRY_SLEEP = 0.01     # 10 ms between retries after a stale read
RETRY_TIMEOUT = 5.0    # give up waiting for consistency after 5 s
PRODUCT_ID = 1


def now_utc():
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%fZ")


def ms_since(start):
    # perf_counter is a precise clock for measuring durations
    return round((time.perf_counter() - start) * 1000, 2)


def create_cart(session, base, customer_id):
    """POST /v1/shopping-carts. Returns the new cart id, or raises on failure."""
    r = session.post(f"{base}/v1/shopping-carts", json={"customer_id": customer_id}, timeout=10)
    r.raise_for_status()  # throws if status is 4xx/5xx
    return r.json()["shopping_cart_id"]


def add_item(session, base, cart_id, quantity=1):
    """POST /v1/shopping-carts/{id}/items. Returns the status code (204 = ok)."""
    r = session.post(
        f"{base}/v1/shopping-carts/{cart_id}/items",
        json={"product_id": PRODUCT_ID, "quantity": quantity},
        timeout=10,
    )
    return r.status_code


def get_quantity(session, base, cart_id):
    """GET /v1/shopping-carts/{id}.

    Returns (status_code, quantity of PRODUCT_ID or 0 if not in the cart).
    """
    r = session.get(f"{base}/v1/shopping-carts/{cart_id}", timeout=10)
    if r.status_code != 200:
        return r.status_code, 0
    for item in r.json().get("items", []):
        if item["product_id"] == PRODUCT_ID:
            return 200, item["quantity"]
    return 200, 0


def wait_until(check):
    """Calls check() until it returns True or RETRY_TIMEOUT passes.

    Returns (became_consistent, retries, ms_waited).
    """
    start = time.perf_counter()
    retries = 0
    while time.perf_counter() - start < RETRY_TIMEOUT:
        time.sleep(RETRY_SLEEP)
        retries += 1
        if check():
            return True, retries, ms_since(start)
    return False, retries, ms_since(start)


def test_create_then_get(session, base):
    """Test 1: create a cart, then GET it immediately, ROUNDS times."""
    records = []
    for i in range(ROUNDS):
        cart_id = create_cart(session, base, customer_id=1000 + i)
        status, _ = get_quantity(session, base, cart_id)
        record = {"round": i + 1, "cart_id": cart_id, "first_get_status": status,
                  "stale": status != 200, "timestamp": now_utc()}
        # 404 right after a successful create = the read missed the new cart
        if record["stale"]:
            ok, retries, waited = wait_until(lambda: get_quantity(session, base, cart_id)[0] == 200)
            record.update({"became_consistent": ok, "retries": retries, "ms_until_consistent": waited})
        records.append(record)
    return records


def test_add_then_get(session, base):
    """Test 2: on one cart, add 1 of PRODUCT_ID then GET immediately, ROUNDS times.

    After round n the quantity should be n; a lower number is a stale read.
    """
    cart_id = create_cart(session, base, customer_id=2000)
    # make sure the cart itself is visible before starting, so test 1 effects don't leak in
    wait_until(lambda: get_quantity(session, base, cart_id)[0] == 200)

    records = []
    for i in range(ROUNDS):
        expected = i + 1
        add_status = add_item(session, base, cart_id)
        status, qty = get_quantity(session, base, cart_id)
        record = {"round": expected, "cart_id": cart_id, "add_status": add_status,
                  "expected_quantity": expected, "first_get_quantity": qty,
                  "stale": qty < expected, "timestamp": now_utc()}
        if record["stale"]:
            ok, retries, waited = wait_until(lambda: get_quantity(session, base, cart_id)[1] >= expected)
            record.update({"became_consistent": ok, "retries": retries, "ms_until_consistent": waited})
        records.append(record)
    return records


def test_concurrent_adds(base):
    """Test 3: CLIENTS threads each add 1 of PRODUCT_ID ADDS_PER_CLIENT times to one cart.

    If every add is applied, the final quantity is CLIENTS * ADDS_PER_CLIENT.
    Anything lower means updates were lost (one write overwrote another).
    """
    setup = requests.Session()
    cart_id = create_cart(setup, base, customer_id=3000)
    wait_until(lambda: get_quantity(setup, base, cart_id)[0] == 200)

    statuses = []
    lock = threading.Lock()  # statuses list is shared by all threads

    def client(_):
        s = requests.Session()  # one connection per client, like separate users
        for _ in range(ADDS_PER_CLIENT):
            code = add_item(s, base, cart_id)
            with lock:
                statuses.append(code)

    start = time.perf_counter()
    # run CLIENTS copies of client() at the same time
    with ThreadPoolExecutor(max_workers=CLIENTS) as pool:
        list(pool.map(client, range(CLIENTS)))
    duration = ms_since(start)

    expected = CLIENTS * ADDS_PER_CLIENT
    _, first_qty = get_quantity(setup, base, cart_id)
    # first read may be stale, so also read again after 1 s to get the settled value
    time.sleep(1)
    _, settled_qty = get_quantity(setup, base, cart_id)
    return {
        "cart_id": cart_id, "clients": CLIENTS, "adds_per_client": ADDS_PER_CLIENT,
        "adds_accepted": statuses.count(204), "adds_failed": len(statuses) - statuses.count(204),
        "expected_quantity": expected, "first_read_quantity": first_qty,
        "settled_quantity": settled_qty, "lost_updates": expected - settled_qty,
        "duration_ms": duration, "timestamp": now_utc(),
    }


def summarize(records):
    stale = [r for r in records if r["stale"]]
    waits = [r["ms_until_consistent"] for r in stale if r.get("became_consistent")]
    return {
        "rounds": len(records),
        "stale_reads": len(stale),
        "never_consistent": sum(1 for r in stale if not r.get("became_consistent")),
        "max_ms_until_consistent": max(waits) if waits else 0,
    }


def main():
    if len(sys.argv) != 4:
        print("Usage: python consistency_test.py <server_url> <label> <output_file>")
        sys.exit(1)
    base, label, output_file = sys.argv[1].rstrip("/"), sys.argv[2], sys.argv[3]
    session = requests.Session()

    print(f"[{label}] test 1: create then get ...")
    t1 = test_create_then_get(session, base)
    print(f"[{label}] test 2: add then get ...")
    t2 = test_add_then_get(session, base)
    print(f"[{label}] test 3: concurrent adds ...")
    t3 = test_concurrent_adds(base)

    results = {
        "label": label, "target": base, "started": t1[0]["timestamp"], "finished": now_utc(),
        "test1_create_then_get": {"summary": summarize(t1), "records": t1},
        "test2_add_then_get": {"summary": summarize(t2), "records": t2},
        "test3_concurrent_adds": t3,
    }
    with open(output_file, "w") as f:
        json.dump(results, f, indent=2)

    print(f"test 1: {results['test1_create_then_get']['summary']}")
    print(f"test 2: {results['test2_add_then_get']['summary']}")
    print(f"test 3: accepted {t3['adds_accepted']}/{t3['expected_quantity']}, "
          f"first read {t3['first_read_quantity']}, settled {t3['settled_quantity']}, "
          f"lost {t3['lost_updates']}")
    print(f"saved to {output_file}")


if __name__ == "__main__":
    main()
