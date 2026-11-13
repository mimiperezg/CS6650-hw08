"""Load test for the shopping cart endpoints.

Each simulated user works on one cart at a time: it creates the cart, adds
products to it, and reads it back. A cart is capped at 50 distinct products,
the largest cart the assignment says the service must handle efficiently;
after that the user starts a new cart.

If creating a cart fails, the user has no cart ID, so it records the failure
and tries to create a cart again on its next task instead of sending
requests for a cart that doesn't exist.

    locust -f cartfile.py CartUser --host http://<HOST>:8080
"""

import random

from locust import FastHttpUser, between, task

MAX_ITEMS_PER_CART = 50


class CartUser(FastHttpUser):
    # Same pacing as the product users in locustfile.py.
    wait_time = between(0.5, 2)

    def on_start(self):
        self.cart_id = None
        self.product_ids = set()
        self.create_cart()

    def create_cart(self):
        self.cart_id = None
        self.product_ids = set()
        with self.client.post(
            "/v1/shopping-carts",
            json={"customer_id": random.randint(1, 10_000)},
            name="/v1/shopping-carts",
            catch_response=True,
        ) as response:
            if response.status_code != 201:
                response.failure(f"expected 201, got {response.status_code}: {response.text}")
                return
            self.cart_id = response.json()["shopping_cart_id"]
            response.success()

    @task
    def add_item(self):
        if self.cart_id is None or len(self.product_ids) >= MAX_ITEMS_PER_CART:
            self.create_cart()
            return

        product_id = random.randint(1, 100_000)
        while product_id in self.product_ids:
            product_id = random.randint(1, 100_000)

        with self.client.post(
            f"/v1/shopping-carts/{self.cart_id}/items",
            json={"product_id": product_id, "quantity": random.randint(1, 5)},
            name="/v1/shopping-carts/[id]/items",
            catch_response=True,
        ) as response:
            if response.status_code == 204:
                self.product_ids.add(product_id)
                response.success()
            else:
                response.failure(f"expected 204, got {response.status_code}: {response.text}")

    @task
    def get_cart(self):
        if self.cart_id is None:
            self.create_cart()
            return

        with self.client.get(
            f"/v1/shopping-carts/{self.cart_id}",
            name="/v1/shopping-carts/[id]",
            catch_response=True,
        ) as response:
            if response.status_code != 200:
                response.failure(f"expected 200, got {response.status_code}: {response.text}")
            elif response.json().get("shopping_cart_id") != self.cart_id:
                response.failure(f"asked for cart {self.cart_id}, got {response.text}")
            else:
                response.success()
