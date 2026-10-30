terraform {
  required_version = ">= 1.10"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.60"
    }
    snowflake = {
      source  = "snowflakedb/snowflake"
      version = "~> 2.20"
    }
  }
}

provider "aws" {
  region = var.region
  default_tags {
    tags = { project = "overhead" }
  }
}

# Credentials from the usual SNOWFLAKE_* environment (SNOWFLAKE_ORGANIZATION_NAME, SNOWFLAKE_ACCOUNT_NAME,
# SNOWFLAKE_USER, SNOWFLAKE_PASSWORD or a key); see README.
provider "snowflake" {
  role = "ACCOUNTADMIN"
}
