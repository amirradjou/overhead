variable "region" {
  description = "AWS region for the bucket and the Fargate task; Snowflake's account should be in the same one."
  type        = string
  default     = "ca-central-1"
}

variable "name" {
  description = "Prefix for every resource's name."
  type        = string
  default     = "overhead"
}

variable "history_prefix" {
  description = "Where in the bucket the history writer puts its batches."
  type        = string
  default     = "history"
}

variable "image_tag" {
  description = "Tag of the overhead image in the ECR repository (make image pushes it)."
  type        = string
  default     = "latest"
}

variable "center" {
  description = "The centre of the circle the poller watches; the default is Toronto Pearson."
  type        = object({ lat = number, lon = number })
  default     = { lat = 43.6777, lon = -79.6248 }
}

variable "timezone" {
  description = "The time zone of \"today\" for the services (they run in UTC otherwise)."
  type        = string
  default     = "America/Toronto"
}

variable "snowflake" {
  description = "Create the Snowflake side (needs SNOWFLAKE_* credentials); false keeps everything in AWS."
  type        = bool
  default     = true
}

variable "budget_email" {
  description = "Where the US$5 monthly cost alert goes (docs/SPEC.md: Cost). make up refuses to run without it."
  type        = string
  default     = ""
}
