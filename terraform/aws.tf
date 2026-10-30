data "aws_caller_identity" "me" {}

locals {
  account_id = data.aws_caller_identity.me.account_id
  bucket     = "${var.name}-history-${local.account_id}"
}

# --- History bucket ----------------------------------------------------------------------------------------------

resource "aws_s3_bucket" "history" {
  bucket        = local.bucket
  force_destroy = true # make down removes the data too: AWS is for tests, the live history stays on the server
}

resource "aws_s3_bucket_public_access_block" "history" {
  bucket                  = aws_s3_bucket.history.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_lifecycle_configuration" "history" {
  bucket = aws_s3_bucket.history.id
  rule {
    id     = "expire-test-history"
    status = "Enabled"
    filter { prefix = "${var.history_prefix}/" }
    expiration { days = 30 }
  }
}

# --- Image -------------------------------------------------------------------------------------------------------

resource "aws_ecr_repository" "overhead" {
  name                 = var.name
  force_delete         = true
  image_tag_mutability = "MUTABLE"
}

# --- Fargate: RabbitMQ, the poller and the history writer in one small task --------------------------------------

resource "aws_ecs_cluster" "overhead" {
  name = var.name
}

resource "aws_cloudwatch_log_group" "overhead" {
  name              = "/ecs/${var.name}"
  retention_in_days = 7
}

data "aws_iam_policy_document" "ecs_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "execution" {
  name               = "${var.name}-execution"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
}

resource "aws_iam_role_policy_attachment" "execution" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

# The task may only add batches under the history prefix.
resource "aws_iam_role" "task" {
  name               = "${var.name}-task"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
}

data "aws_iam_policy_document" "write_history" {
  statement {
    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.history.arn}/${var.history_prefix}/*"]
  }
}

resource "aws_iam_role_policy" "write_history" {
  name   = "write-history"
  role   = aws_iam_role.task.id
  policy = data.aws_iam_policy_document.write_history.json
}

locals {
  image = "${aws_ecr_repository.overhead.repository_url}:${var.image_tag}"
  env = [
    { name = "OVERHEAD_AMQP", value = "amqp://overhead:overhead@localhost:5672/" },
    { name = "OVERHEAD_LAT", value = tostring(var.center.lat) },
    { name = "OVERHEAD_LON", value = tostring(var.center.lon) },
    { name = "AWS_REGION", value = var.region },
  ]
  logs = {
    logDriver = "awslogs"
    options = {
      awslogs-group         = aws_cloudwatch_log_group.overhead.name
      awslogs-region        = var.region
      awslogs-stream-prefix = "overhead"
    }
  }
}

resource "aws_ecs_task_definition" "overhead" {
  family                   = var.name
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = 512
  memory                   = 1024
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task.arn
  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "X86_64"
  }
  container_definitions = jsonencode([
    {
      name             = "rabbitmq"
      image            = "rabbitmq:4.1-alpine"
      essential        = true
      environment      = [{ name = "RABBITMQ_DEFAULT_USER", value = "overhead" }, { name = "RABBITMQ_DEFAULT_PASS", value = "overhead" }]
      logConfiguration = local.logs
      healthCheck = {
        command  = ["CMD", "rabbitmq-diagnostics", "-q", "ping"]
        interval = 10
        retries  = 6
        timeout  = 10
      }
    },
    {
      name             = "poll"
      image            = local.image
      essential        = true
      command          = ["poll"]
      environment      = local.env
      logConfiguration = local.logs
      dependsOn        = [{ containerName = "rabbitmq", condition = "HEALTHY" }]
    },
    {
      name             = "history"
      image            = local.image
      essential        = true
      command          = ["history", "-s3", "s3://${aws_s3_bucket.history.bucket}/${var.history_prefix}"]
      environment      = local.env
      logConfiguration = local.logs
      dependsOn        = [{ containerName = "rabbitmq", condition = "HEALTHY" }]
    },
  ])
}

# The default VPC's public subnets with egress only: the task calls the feeds and S3, nothing calls it.
data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

resource "aws_security_group" "task" {
  name        = "${var.name}-task"
  description = "overhead task: outbound only"
  vpc_id      = data.aws_vpc.default.id
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_ecs_service" "overhead" {
  name            = var.name
  cluster         = aws_ecs_cluster.overhead.id
  task_definition = aws_ecs_task_definition.overhead.arn
  desired_count   = 1
  launch_type     = "FARGATE"
  network_configuration {
    subnets          = data.aws_subnets.default.ids
    security_groups  = [aws_security_group.task.id]
    assign_public_ip = true # no NAT gateway: a public IP is the cheap way out to the internet
  }
}
