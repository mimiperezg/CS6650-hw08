# One table for shopping carts: one item per cart, keyed by cart_id.
# Cart products live inside the item as a map, so no sort key is needed.
resource "aws_dynamodb_table" "carts" {
  name = var.table_name
  # on-demand: pay per request, no read/write capacity to plan
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "cart_id" # partition key

  # only key attributes are declared here; the rest (customer_id, items, ...)
  # are schemaless and just appear on the item when the app writes them
  attribute {
    name = "cart_id"
    type = "N" # number
  }

  # dynamodb deletes a cart on its own once the epoch-seconds time in
  # expires_at has passed (carts are session data that expire naturally)
  ttl {
    attribute_name = "expires_at"
    enabled        = true
  }
}
