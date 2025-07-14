# CDKTF VPC Peering Module

This repository provides a reusable [CDK for Terraform (CDKTF)](https://developer.hashicorp.com/terraform/cdktf) module for managing AWS VPC peering connections, including bi-directional routing, DNS resolution, and automatic subnet route management. It supports cross-account and cross-region peering with explicit accepter resources.

---

## Features

- **Automated VPC peering** between multiple AWS accounts/regions
- **Bi-directional main and subnet route table management**
- **DNS resolution options**
- **Automatic handling of main vs. subnet route tables**
- **YAML-driven configuration for easy management**

---

## Usage

### 1. Prerequisites

- [Go](https://golang.org/)
- [Node.js](https://nodejs.org/)
- [Terraform](https://terraform.io/)
- [CDKTF CLI](https://developer.hashicorp.com/terraform/cdktf)

### 2. Setup

```sh
npm install
cdktf get
cdktf init --template=go
terraform init
```

### 3. Configuration

Create a `peering.yaml` file in the repo root. Example:

```yaml
peers:
  example-peer:
    vpc_id: vpc-0123456789abcdef0
    region: us-west-2
    role_arn: "arn:aws:iam::123456789012:role/ExampleRole"
    dns_resolution: true
    has_additional_routes: true

  another-peer:
    vpc_id: vpc-0fedcba9876543210
    region: us-west-2
    role_arn: "arn:aws:iam::210987654321:role/AnotherRole"
    dns_resolution: true
    has_additional_routes: false

peering_matrix:
  example-peer:
    - another-peer
```

- Add as many peers and matrix entries as needed.
- The `peering_matrix` defines which peers should be connected.

---

## Common Commands

- `make init`      – Install dependencies and initialize cdktf/terraform
- `make synth`     – Synthesize the cdktf stack to Terraform JSON
- `make plan`      – Run terraform plan using the synthesized stack
- `make apply`     – Run terraform apply using the synthesized stack
- `make destroy`   – Destroy the deployed resources
- `make clean`     – Remove build artifacts

---

## Notes

- Set the `CDKTF_SOURCE` environment variable to filter which peer(s) to use as the source for peering.
- See `main.go` and `helpers.go` for implementation details and extensibility.

---

## License

MIT
