.PHONY: build test lint fmt up down image site

build:
	go build -o bin/overhead ./cmd/overhead

test:
	go test -race ./...

lint:
	go vet ./...
	test -z "$$(gofmt -l .)"

fmt:
	gofmt -w .

# M6: the AWS and Snowflake side (docs/SPEC.md: Infra). AWS credentials and region as usual; SNOWFLAKE_* for Snowflake,
# or TF_VAR_snowflake=false to leave it out. `make down` removes everything, data included.
TF := terraform -chdir=terraform

up:
	@test -n "$$TF_VAR_budget_email" || { echo "set TF_VAR_budget_email: the US\$$5 cost alert goes up first"; exit 1; }
	$(TF) init -input=false
	$(TF) apply -input=false -auto-approve
	$(MAKE) image
	aws ecs update-service --cluster $$($(TF) output -raw cluster) --service $$($(TF) output -raw service) \
		--force-new-deployment > /dev/null

down:
	$(TF) destroy -input=false -auto-approve

image:
	repo=$$($(TF) output -raw image_repository) && \
	aws ecr get-login-password | docker login --username AWS --password-stdin $${repo%%/*} && \
	docker build --build-arg VERSION=$$(git describe --always --dirty) -t $$repo:latest . && \
	docker push $$repo:latest

# The public page (docs/setup.md, section 6): the screen as a static site that reads the public instance's stream and
# plays its recording while that is away. OVERHEAD_SITE is the Netlify site's ID (overhead-window).
OVERHEAD_PUBLIC_API ?= https://framework.tail420820.ts.net:8443
OVERHEAD_SITE ?= eba2d9ba-6e75-44bb-9561-898079c5d503

site:
	cd web && NEXT_PUBLIC_OVERHEAD_API=$(OVERHEAD_PUBLIC_API) NEXT_PUBLIC_OVERHEAD_REPLAY=1 pnpm build
	sed -i 's|OVERHEAD_PUBLIC_API|$(OVERHEAD_PUBLIC_API)|' web/out/_headers
	cd web && netlify deploy --prod --no-build --dir out --site $(OVERHEAD_SITE)
