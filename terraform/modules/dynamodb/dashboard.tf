# Monitoring for the carts table (Step II Part 6).

# Contributor Insights: dynamodb tracks which cart_id keys get the most
# reads/writes, so we can check no single key (partition) is hot.
# View it in the DynamoDB console: table -> Monitor -> Contributor Insights.
resource "aws_dynamodb_contributor_insights" "carts" {
  table_name = aws_dynamodb_table.carts.name
}

# region the table lives in (needed by every dashboard widget)
data "aws_region" "current" {}

locals {
  table  = aws_dynamodb_table.carts.name
  region = data.aws_region.current.region
  # the 3 operations the cart api uses: create = PutItem, get = GetItem, add item = UpdateItem
  operations = ["GetItem", "PutItem", "UpdateItem"]
}

# One dashboard with a panel per thing part 6 asks for. All panels use
# 1-minute data points (period = 60) so short tests are still visible.
resource "aws_cloudwatch_dashboard" "dynamodb" {
  dashboard_name = "hw08-dynamodb"

  dashboard_body = jsonencode({
    widgets = [
      {
        # request latency inside dynamodb (does not include network to the app)
        type = "metric", x = 0, y = 0, width = 12, height = 6
        properties = {
          title  = "Request latency (ms) per operation"
          region = local.region
          period = 60
          stat   = "Average"
          # one line per operation: [namespace, metric, dim name, dim value, ...]
          metrics = [for op in local.operations :
            ["AWS/DynamoDB", "SuccessfulRequestLatency", "TableName", local.table, "Operation", op]
          ]
        }
      },
      {
        # read/write capacity units used (on-demand bills per unit)
        type = "metric", x = 12, y = 0, width = 12, height = 6
        properties = {
          title  = "Consumed capacity units"
          region = local.region
          period = 60
          stat   = "Sum"
          metrics = [
            ["AWS/DynamoDB", "ConsumedReadCapacityUnits", "TableName", local.table],
            ["AWS/DynamoDB", "ConsumedWriteCapacityUnits", "TableName", local.table],
          ]
        }
      },
      {
        # requests rejected because a partition was over its limit (expect 0)
        type = "metric", x = 0, y = 6, width = 12, height = 6
        properties = {
          title  = "Throttling events"
          region = local.region
          period = 60
          stat   = "Sum"
          metrics = [
            ["AWS/DynamoDB", "ReadThrottleEvents", "TableName", local.table],
            ["AWS/DynamoDB", "WriteThrottleEvents", "TableName", local.table],
          ]
        }
      },
      {
        # errors by type:
        #   SystemErrors = aws-side failures (500s)
        #   UserErrors = bad requests (400s); this metric is per account+region, not per table
        #   ConditionalCheckFailedRequests = writes refused by a condition (duplicate cart id, add to missing cart)
        type = "metric", x = 12, y = 6, width = 12, height = 6
        properties = {
          title  = "Errors by type"
          region = local.region
          period = 60
          stat   = "Sum"
          metrics = concat(
            [for op in local.operations :
              ["AWS/DynamoDB", "SystemErrors", "TableName", local.table, "Operation", op]
            ],
            [
              ["AWS/DynamoDB", "UserErrors"],
              ["AWS/DynamoDB", "ConditionalCheckFailedRequests", "TableName", local.table],
            ]
          )
        }
      },
    ]
  })
}
