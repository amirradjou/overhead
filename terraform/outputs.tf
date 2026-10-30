output "bucket" {
  value = aws_s3_bucket.history.bucket
}

output "history_url" {
  description = "Where the history writer puts its batches (overhead history -s3 ...)."
  value       = "s3://${aws_s3_bucket.history.bucket}/${var.history_prefix}"
}

output "image_repository" {
  description = "Push the image here (make image)."
  value       = aws_ecr_repository.overhead.repository_url
}

output "snowflake_pipe" {
  value = var.snowflake ? snowflake_pipe.positions[0].fully_qualified_name : null
}
