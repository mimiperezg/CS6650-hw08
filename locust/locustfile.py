"""Load test for the Product Service.

Each simulated user adds products with POST and reads back the ones it
added with GET, so a 404 on GET is a real failure rather than a product
that was never created.

The same tasks run on two HTTP clients. Pick one by class name:

    locust ProductHttpUser       # requests-based client (Locust's default)
    locust ProductFastHttpUser   # geventhttpclient-based, much less CPU per request
"""

import random

from locust import FastHttpUser, HttpUser, User, between, task


class ProductUser(User):
    # Abstract: Locust never runs this class itself, only the subclasses
    # below, which add the HTTP client the tasks call through self.client.
    abstract = True

    # Pause 0.5-2 seconds between tasks, like a real client would.
    wait_time = between(0.5, 2)

    def on_start(self):
        self.product_ids = []

    @task(1)
    def add_product(self):
        product_id = random.randint(1, 100_000)
        product = {
            "product_id": product_id,
            "sku": f"SKU-{product_id}",
            "manufacturer": "Acme Corporation",
            "category_id": random.randint(1, 50),
            "weight": random.randint(0, 5000),
            "some_other_id": random.randint(1, 1000),
        }
        # name= groups every ID under one row in Locust's stats.
        with self.client.post(
            f"/v1/products/{product_id}/details",
            json=product,
            name="/v1/products/[id]/details",
            catch_response=True,
        ) as response:
            if response.status_code == 204:
                self.product_ids.append(product_id)
                response.success()
            else:
                response.failure(f"expected 204, got {response.status_code}: {response.text}")

    # Reads are weighted 3:1 over writes.
    @task(3)
    def get_product(self):
        if not self.product_ids:
            self.add_product()
            return

        product_id = random.choice(self.product_ids)
        with self.client.get(
            f"/v1/products/{product_id}",
            name="/v1/products/[id]",
            catch_response=True,
        ) as response:
            if response.status_code != 200:
                response.failure(f"expected 200, got {response.status_code}: {response.text}")
            elif response.json().get("product_id") != product_id:
                response.failure(f"asked for product {product_id}, got {response.text}")
            else:
                response.success()


class ProductHttpUser(ProductUser, HttpUser):
    pass


class ProductFastHttpUser(ProductUser, FastHttpUser):
    pass
