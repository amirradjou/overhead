.PHONY: build test lint fmt up down image

build:
	go build -o bin/overhead ./cmd/overhead

test:
	go test -race ./...

lint:
	go vet ./...
	test -z "$$(gofmt -l .)"

fmt:
	gofmt -w .

# M6: the AWS and Snowflake side (README: Infra). AWS credentials and region as usual; SNOWFLAKE_* for Snowflake,
# or TF_VAR_snowflake=false to leave it out. `make down` removes everything, data included.
TF := terraform -chdir=terraform

up:
	$(TF) init -input=false
	$(TF) apply -input=false -auto-approve
	$(MAKE) image
	aws ecs update-service --cluster overhead --service overhead --force-new-deployment > /dev/null

down:
	$(TF) destroy -input=false -auto-approve

image:
	repo=$$($(TF) output -raw image_repository) && \
	aws ecr get-login-password | docker login --username AWS --password-stdin $${repo%%/*} && \
	docker build --build-arg VERSION=$$(git describe --always --dirty) -t $$repo:latest . && \
	docker push $$repo:latest
