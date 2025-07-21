# ------------------------------------------------------------------------------
# Makefile for CDKTF (Go) project with AWS provider
# ------------------------------------------------------------------------------

# --- Stack and Provider Versions ---
STACK_NAME ?= cdktf-vpc-peering-module
AWS_PROVIDER_VERSION ?= 5.42.0
GEN_PATH := .gen/hashicorp/aws
REPLACE_DIRECTIVE := replace cdk.tf/go/stack/generated/hashicorp/aws => ./${GEN_PATH}

# --- Silence unsupported Node warnings from JSII ---
export JSII_SILENCE_WARNING_UNTESTED_NODE_VERSION=true

.PHONY: init provider fix-replace get tidy synth deploy destroy clean build check

# ------------------------------------------------------------------------------
#  Initialization
# ------------------------------------------------------------------------------

# --- One-time init (don't run in non-empty dirs) ---
init:
	@echo "Skip 'cdktf init' in non-empty directory."

# ------------------------------------------------------------------------------
#  Dependency Management
# ------------------------------------------------------------------------------

# --- Generate provider bindings ---
get:
	CDKTF_LOG_LEVEL=info cdktf get

# --- Install Go deps ---
tidy:
	go mod tidy

# ------------------------------------------------------------------------------
#  Code Quality
# ------------------------------------------------------------------------------

# --- Format Go code ---
fmt:
	@echo "==> gofmt (root)..."
	@if [ -f go.mod ]; then find . -type f -name '*.go' -not -path './generated/*' -exec gofmt -s -w {} +; fi

# --- Lint Go code ---
lint:
	@echo "==> golint (root)..."
	@if [ -f go.mod ]; then golint $(find . -type f -name '*.go' -not -path './generated/*'); fi

# --- Run Go tests ---
test:
	@echo "==> go test (root)..."
	@if [ -f go.mod ]; then gotestsum --format=testname $(shell go list ./... | grep -v 'generated'); fi

# ------------------------------------------------------------------------------
#  Synthesis & Deployment
# ------------------------------------------------------------------------------

# --- Compile and synthesize Terraform config ---
synth:
	cdktf synth -- -source=openvpn-as-production-us-east-1

# --- Deploy (terraform apply) ---
deploy:
	cdktf deploy --auto-approve

# --- Plan (terraform plan) ---
plan:
	cdktf plan --auto-approve -- -source=openvpn-as-production-us-east-1

# --- Destroy resources ---
destroy:
	cdktf destroy --auto-approve

# ------------------------------------------------------------------------------
#  Build & Clean
# ------------------------------------------------------------------------------

# --- Clean up generated files ---
clean:
	rm -rf cdktf.out .gen && find . -name "*.terraform*" -exec rm -rf {} +

# --- Full fresh build ---
build: clean get tidy synth plan sec

# ------------------------------------------------------------------------------
#  Terraform CLI
# ------------------------------------------------------------------------------

# --- Plan using raw terraform CLI ---
terraform-shell:
	cd cdktf.out/stacks/$(STACK_NAME) && terraform init && terraform plan

# ------------------------------------------------------------------------------
#  Security
# ------------------------------------------------------------------------------

# --- Run all security checks ---
sec: trivy checkov

# --- Run Trivy security scan ---
trivy:
	trivy config --skip-dirs generated --skip-dirs .gen .

# --- Run Checkov security scan ---
checkov:
	checkov -d . --skip-path generated --skip-path .gen

# ------------------------------------------------------------------------------
#  Utilities
# ------------------------------------------------------------------------------

# --- List all Makefile tasks ---
list:
	@awk '/^[a-zA-Z0-9_-]+:/ && !/^\./ {print $$1}' $(MAKEFILE_LIST) | sed 's/://'

