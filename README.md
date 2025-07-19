# CDKTF VPC Peering Module

This repository provides a reusable [CDK for Terraform (CDKTF)](https://developer.hashicorp.com/terraform/cdktf) module for managing AWS VPC peering connections. It automates bi-directional routing, DNS resolution, and subnet route management for scalable, multi-account, and multi-region AWS environments. The module supports cross-account and cross-region peering with explicit accepter resources, and is driven by a simple YAML configuration.

---

## Features

- **Automated VPC peering** between multiple AWS accounts and regions
- **Bi-directional main and subnet route table management** for seamless connectivity
- **DNS resolution options** for cross-VPC name resolution
- **Automatic handling of main vs. subnet route tables**
- **YAML-driven configuration** for easy, declarative management
- **Cross-account and cross-region support** with explicit accepter resources
- **Extensible Go codebase** for advanced customization

---

## Usage

### 1. Prerequisites

- [Go](https://golang.org/)
- [Node.js](https://nodejs.org/)
- [Terraform](https://terraform.io/)
- [CDKTF CLI](https://developer.hashicorp.com/terraform/cdktf)

### 2. Setup

```sh
# Install Go and Node.js dependencies if needed
make init

# Generate provider bindings
make get

# Initialize Terraform and CDKTF
cdktf init --template=go
terraform init
```

### 3. Configuration

Create a `peering.yaml` file in the repo root. Example:

```yaml
peers:
  dev-peer:
    vpc_id: vpc-0aaa1111aaa1111aa
    region: us-east-1
    role_arn: "arn:aws:iam::111111111111:role/DevRole"
    dns_resolution: true
    has_additional_routes: false

  prod-peer:
    vpc_id: vpc-0bbb2222bbb2222bb
    region: us-west-2
    role_arn: "arn:aws:iam::222222222222:role/ProdRole"
    dns_resolution: true
    has_additional_routes: true

  staging-peer:
    vpc_id: vpc-0ccc3333ccc3333cc
    region: us-east-2
    role_arn: "arn:aws:iam::333333333333:role/StagingRole"
    dns_resolution: false
    has_additional_routes: false

  qa-peer:
    vpc_id: vpc-0ddd4444ddd4444dd
    region: us-west-1
    role_arn: "arn:aws:iam::444444444444:role/QARole"
    dns_resolution: true
    has_additional_routes: true

peering_matrix:
  dev-peer:
    - prod-peer
    - staging-peer
  qa-peer:
    - prod-peer
```

- Add as many peers and matrix entries as needed.
- The `peering_matrix` defines which peers should be connected to which others.
- Each peer can have custom DNS and route table options.

---

## Common Commands

- `make init`      – Install dependencies and initialize cdktf/terraform
- `make get`       – Generate provider bindings
- `make synth`     – Synthesize the cdktf stack to Terraform JSON
- `make plan`      – Run terraform plan using the synthesized stack
- `make apply`     – Run terraform apply using the synthesized stack
- `make destroy`   – Destroy the deployed resources
- `make clean`     – Remove build artifacts

---

## Notes

- Set the `CDKTF_SOURCE` environment variable to filter which peer(s) to use as the source for peering.
- See `main.go` and `helpers.go` for implementation details and extensibility.
- Security and linting checks are available via `make sec` and `make golint`.

---

## License

MIT
