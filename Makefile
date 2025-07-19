# ------------------------------------------------------------------------------
# Makefile for CDKTF (Go) project with AWS provider
# ------------------------------------------------------------------------------

STACK_NAME ?= cdktf-vpc-peering-module
AWS_PROVIDER_VERSION ?= 5.42.0
GEN_PATH := .gen/hashicorp/aws
REPLACE_DIRECTIVE := replace cdk.tf/go/stack/generated/hashicorp/aws => ./${GEN_PATH}

# Export to silence unsupported Node warnings from JSII
export JSII_SILENCE_WARNING_UNTESTED_NODE_VERSION=true

.PHONY: init provider fix-replace get tidy synth deploy destroy clean build check

# One-time init (don't run in non-empty dirs)
init:
	@echo "Skip 'cdktf init' in non-empty directory."

# Generate provider bindings
get:
	cdktf get

# Install Go deps
tidy:
	go mod tidy

# Compile and synthesize Terraform config
synth:
	cdktf synth -- -source=openvpn-as-production-us-east-1

# Deploy (terraform apply)
deploy:
	cdktf deploy --auto-approve

# Deploy (terraform apply)
plan:
	cdktf plan --auto-approve -- -source=openvpn-as-production-us-east-1

# Destroy resources
destroy:
	cdktf destroy --auto-approve

# Clean up generated files
clean:
	rm -rf cdktf.out .gen && find . -name "*.terraform*" -exec rm -rf {} +

# Full fresh build
build: clean get tidy synth plan

# Plan using raw terraform CLI
terraform-shell:
	cd cdktf.out/stacks/$(STACK_NAME) && terraform init && terraform plan

# ------------------------------------------------------
#  Security
# ------------------------------------------------------

sec: trivy checkov

trivy:
	trivy config .

checkov:
	checkov -d .

# ------------------------------------------------------
#  Unit tests (Terraform native)
#   Runs 'terraform test' for native module/unit tests.
# ------------------------------------------------------

test:
	terraform test -no-color

gofmt:
	@echo "==> gofmt (root)..."
	@if [ -f go.mod ]; then find . -type f -name '*.go' -not -path './generated/*' -exec gofmt -s -w {} +; fi

golint:
	@echo "==> golint (root)..."
	@if [ -f go.mod ]; then golint $(find . -type f -name '*.go' -not -path './generated/*'); fi
