import json
import sys
import time
from datetime import datetime, timezone
import requests

ROUNDS = 50

def timed_request(session, method, url, operation, expected_status, body=None):
    """sends one request, times it,  and return (record, response)
    
    the response is None if request never got answered
    """

    # timestamp the request in UTC
    timestamp = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

    # start. perf_counter for more accurate timing used for durations
    start = time.perf_counter()
    try:
        response = session.request(method, url, json=body, timeout=10)
        status_code = response.status_code
    except requests.RequestException:
        # connection refused
        response = None
        status_code = 0
    # go from seconds to milliseconds and round to 2 decimals
    response_time = round((time.perf_counter() - start) * 1000, 2)

    record = {
        "operation": operation,
        "status_code": status_code,
        "response_time": response_time,
        "success": status_code == expected_status,
        "timestamp": timestamp
    }
    return record, response

def skipped_record(operation):
    """
    this is the record for an operation that couldnt run because a create failed
    """

    return {
        "operation": operation,
        "status_code": 0,
        "response_time": 0,
        "success": False,
        "timestamp": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    }


def main():
    # read the server URL and output file name from commmand line
    if len(sys.argv) != 3:
        print("Usage: python loadtest.py <server_url> <output_file>")
        sys.exit(1)
    base_url = sys.argv[1].rstrip("/")
    output_file = sys.argv[2]

    results = []
    session = requests.Session()

    test_start = time.perf_counter()

    for i in range(ROUNDS):
        customer_id = i + 1
        product_id = (i % 10) + 1
        quantity = 1

        # create cart
        record, response = timed_request(
            session, 
            "POST", 
            f"{base_url}/v1/shopping-carts", 
            "create_cart", 
            201,
            {"customer_id": customer_id}
        )
        results.append(record)

        # without card ID we cant add or get to cart
        if record["success"] == False:
            print(f"Round {i + 1}: create failed (status {record['status_code']}), skipping add/get")
            results.append(skipped_record("add_items"))
            results.append(skipped_record("get_cart"))
            continue

        cart_id = response.json()["shopping_cart_id"]

        # add an item to cart: 204
        record, _ = timed_request(
            session,
            "POST",
            f"{base_url}/v1/shopping-carts/{cart_id}/items",
            "add_items",
            204,
            {"product_id": product_id, "quantity": quantity}
        )
        results.append(record)

        # get the cart: 200
        record, _ = timed_request(
            session,
            "GET",
            f"{base_url}/v1/shopping-carts/{cart_id}",
            "get_cart",
            200,
            None
        )
        results.append(record)

    total_seconds = time.perf_counter() - test_start

    # save the 150 records to json array
    with open(output_file, "w") as f:
        json.dump(results, f, indent=2)

    # summary of run
    successes = sum(1 for r in results if r["success"])
    print(f"Wrote {len(results)} records to {output_file}")
    print(f"Succeeded: {successes}/{len(results)}")
    print(f"Total time: {total_seconds:.2f} seconds")


if __name__ == "__main__":
    main()

