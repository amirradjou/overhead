# The Snowflake side of M3 (snowflake/01_load.sql by hand does the same): an X-Small warehouse that suspends after a
# minute, a storage integration that reads the bucket through an IAM role, and a pipe that loads each new batch as
# S3 announces it. Off with -var snowflake=false.

locals {
  sf            = var.snowflake ? 1 : 0
  sf_name       = upper(var.name)
  stage_url     = "s3://${aws_s3_bucket.history.bucket}/${var.history_prefix}/positions/"
  sf_role_name  = "${var.name}-snowflake-read"
  sf_role_arn   = "arn:aws:iam::${local.account_id}:role/${local.sf_role_name}" # known before the role exists
  position_cols = <<-EOT
    $1:run::STRING, $1:seq::NUMBER, $1:published::TIMESTAMP_TZ, $1:hex::STRING, $1:callsign::STRING,
    $1:registration::STRING, $1:type_code::STRING, $1:squawk::STRING, $1:emergency::STRING,
    $1:lat::FLOAT, $1:lon::FLOAT, $1:alt_ft::NUMBER, COALESCE($1:on_ground::BOOLEAN, FALSE),
    $1:speed_kt::FLOAT, $1:track_deg::FLOAT, $1:vert_rate_fpm::NUMBER,
    COALESCE($1:military::BOOLEAN, FALSE), COALESCE($1:private::BOOLEAN, FALSE),
    $1:source::STRING, $1:at::TIMESTAMP_TZ, METADATA$FILENAME
  EOT
}

resource "snowflake_warehouse" "xs" {
  count               = local.sf
  name                = "${local.sf_name}_XS"
  warehouse_size      = "XSMALL"
  auto_suspend        = 60
  auto_resume         = "true"
  initially_suspended = true
}

resource "snowflake_database" "overhead" {
  count = local.sf
  name  = local.sf_name
}

resource "snowflake_schema" "raw" {
  count    = local.sf
  database = snowflake_database.overhead[0].name
  name     = "RAW"
}

resource "snowflake_storage_integration_aws" "s3" {
  count                     = local.sf
  name                      = "${local.sf_name}_S3"
  enabled                   = true
  storage_provider          = "S3"
  storage_aws_role_arn      = local.sf_role_arn
  storage_allowed_locations = [local.stage_url]
}

# The role Snowflake assumes to read the bucket; it trusts only the integration's own IAM user and external id.
data "aws_iam_policy_document" "snowflake_trust" {
  count = local.sf
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "AWS"
      identifiers = [snowflake_storage_integration_aws.s3[0].describe_output[0].iam_user_arn]
    }
    condition {
      test     = "StringEquals"
      variable = "sts:ExternalId"
      values   = [snowflake_storage_integration_aws.s3[0].describe_output[0].external_id]
    }
  }
}

resource "aws_iam_role" "snowflake" {
  count              = local.sf
  name               = local.sf_role_name
  assume_role_policy = data.aws_iam_policy_document.snowflake_trust[0].json
}

data "aws_iam_policy_document" "snowflake_read" {
  statement {
    actions   = ["s3:GetObject", "s3:GetObjectVersion"]
    resources = ["${aws_s3_bucket.history.arn}/${var.history_prefix}/positions/*"]
  }
  statement {
    actions   = ["s3:ListBucket", "s3:GetBucketLocation"]
    resources = [aws_s3_bucket.history.arn]
  }
}

resource "aws_iam_role_policy" "snowflake_read" {
  count  = local.sf
  name   = "read-history"
  role   = aws_iam_role.snowflake[0].id
  policy = data.aws_iam_policy_document.snowflake_read.json
}

resource "snowflake_file_format_json" "jsonl_gz" {
  count       = local.sf
  database    = snowflake_database.overhead[0].name
  schema      = snowflake_schema.raw[0].name
  name        = "JSONL_GZ"
  compression = "GZIP"
}

resource "snowflake_stage_external_s3" "positions" {
  count               = local.sf
  database            = snowflake_database.overhead[0].name
  schema              = snowflake_schema.raw[0].name
  name                = "POSITIONS_STAGE"
  url                 = local.stage_url
  storage_integration = snowflake_storage_integration_aws.s3[0].name
  file_format {
    format_name = snowflake_file_format_json.jsonl_gz[0].fully_qualified_name
  }
  depends_on = [aws_iam_role_policy.snowflake_read]
}

resource "snowflake_table" "positions" {
  count    = local.sf
  database = snowflake_database.overhead[0].name
  schema   = snowflake_schema.raw[0].name
  name     = "POSITIONS"
  dynamic "column" {
    for_each = [
      ["RUN", "VARCHAR"], ["SEQ", "NUMBER(38,0)"], ["PUBLISHED", "TIMESTAMP_TZ"], ["HEX", "VARCHAR"],
      ["CALLSIGN", "VARCHAR"], ["REGISTRATION", "VARCHAR"], ["TYPE_CODE", "VARCHAR"], ["SQUAWK", "VARCHAR"],
      ["EMERGENCY", "VARCHAR"], ["LAT", "FLOAT"], ["LON", "FLOAT"], ["ALT_FT", "NUMBER(38,0)"],
      ["ON_GROUND", "BOOLEAN"], ["SPEED_KT", "FLOAT"], ["TRACK_DEG", "FLOAT"], ["VERT_RATE", "NUMBER(38,0)"],
      ["MILITARY", "BOOLEAN"], ["PRIVATE", "BOOLEAN"], ["SOURCE", "VARCHAR"], ["AT", "TIMESTAMP_TZ"],
      ["FILE", "VARCHAR"],
    ]
    content {
      name = column.value[0]
      type = column.value[1]
    }
  }
}

resource "snowflake_pipe" "positions" {
  count       = local.sf
  database    = snowflake_database.overhead[0].name
  schema      = snowflake_schema.raw[0].name
  name        = "POSITIONS_PIPE"
  auto_ingest = true
  copy_statement = trimspace(<<-EOT
    COPY INTO ${snowflake_table.positions[0].fully_qualified_name}
      (RUN, SEQ, PUBLISHED, HEX, CALLSIGN, REGISTRATION, TYPE_CODE, SQUAWK, EMERGENCY, LAT, LON, ALT_FT, ON_GROUND,
       SPEED_KT, TRACK_DEG, VERT_RATE, MILITARY, PRIVATE, SOURCE, AT, FILE)
    FROM (SELECT ${trimspace(local.position_cols)} FROM @${snowflake_stage_external_s3.positions[0].fully_qualified_name})
  EOT
  )
  lifecycle {
    replace_triggered_by = [snowflake_stage_external_s3.positions[0].url, snowflake_stage_external_s3.positions[0].storage_integration]
  }
}

# Each new batch under positions/ tells the pipe's queue, and Snowpipe loads it.
resource "aws_s3_bucket_notification" "snowpipe" {
  count  = local.sf
  bucket = aws_s3_bucket.history.id
  queue {
    queue_arn     = snowflake_pipe.positions[0].notification_channel
    events        = ["s3:ObjectCreated:*"]
    filter_prefix = "${var.history_prefix}/positions/"
    filter_suffix = ".jsonl.gz"
  }
}
