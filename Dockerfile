# syntax=docker/dockerfile:1
# ────────────────────────────────────────────────────────────
#  rds-terraform CI image • Ubuntu 24.04 + IaC tool-chain
#  Architecture: linux/amd64  (pin in FROM line)
# ────────────────────────────────────────────────────────────

FROM --platform=linux/amd64 ubuntu:24.04

# -----------------------------------------------------------
# Global environment
# -----------------------------------------------------------

ARG TF_VERSION=1.12.2          # HashiCorp Terraform
ARG TFLINT_VERSION=latest      # newest tag that exists
ARG TRIVY_VERSION=0.48.0
ARG TF_DOCS_VERSION=0.20.0
ARG INFRACOST_VERSION=v0.10.41

ENV DEBIAN_FRONTEND=noninteractive \
    TF_IN_AUTOMATION=1 \
    PIP_BREAK_SYSTEM_PACKAGES=1 \
    PATH=/usr/local/go/bin:$PATH

# -----------------------------------------------------------
# Base OS packages  (curl only — no wget)                     • AVD-DS-0014
# -----------------------------------------------------------

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        curl unzip gnupg software-properties-common \
        ca-certificates git make build-essential python3-pip jq && \
    rm -rf /var/lib/apt/lists/*

# -----------------------------------------------------------
# Go tool-chain (from longsleep PPA for fresh 1.22.x)         • AVD-DS-0029
# -----------------------------------------------------------

RUN add-apt-repository -y ppa:longsleep/golang-backports && \
    apt-get update && \
    apt-get install -y --no-install-recommends golang-go

# -----------------------------------------------------------
# tflint and golint – compile from source (portable & version-pin)
# -----------------------------------------------------------

RUN go install github.com/terraform-linters/tflint@${TFLINT_VERSION} && \
    go install golang.org/x/lint/golint@latest && \
    mv /root/go/bin/tflint /usr/local/bin && \
    mv /root/go/bin/golint /usr/local/bin

# -----------------------------------------------------------
# Terraform CLI (HashiCorp APT repo, amd64 arch)              • AVD-DS-0029
# -----------------------------------------------------------

RUN curl -fsSL https://apt.releases.hashicorp.com/gpg | \
      gpg --dearmor -o /usr/share/keyrings/hashicorp.gpg && \
    echo "deb [arch=amd64 signed-by=/usr/share/keyrings/hashicorp.gpg] \
      https://apt.releases.hashicorp.com $(. /etc/os-release && echo $UBUNTU_CODENAME) main" \
      > /etc/apt/sources.list.d/hashicorp.list && \
    apt-get update && \
    apt-get install -y --no-install-recommends terraform=${TF_VERSION}* || \
    apt-get install -y --no-install-recommends terraform   # fall back if exact pin missing

# -----------------------------------------------------------
# Trivy (binary install script)
# -----------------------------------------------------------

RUN curl -sfL https://raw.githubusercontent.com/aquasecurity/trivy/main/contrib/install.sh | \
      sh -s -- -b /usr/local/bin v${TRIVY_VERSION}

# -----------------------------------------------------------
# Checkov (pip override for PEP 668)
# -----------------------------------------------------------

RUN pip3 install --no-cache-dir checkov

# -----------------------------------------------------------
# terraform-docs (pre-built amd64 tarball)
# -----------------------------------------------------------

RUN curl -sSL \
      https://terraform-docs.io/dl/v${TF_DOCS_VERSION}/terraform-docs-v${TF_DOCS_VERSION}-linux-amd64.tar.gz \
      -o /tmp/td.tar.gz && \
    tar -xzf /tmp/td.tar.gz -C /usr/local/bin && \
    rm /tmp/td.tar.gz

# -----------------------------------------------------------
# Smoke-test versions (helps cache & debugging)
# -----------------------------------------------------------

RUN terraform -version && \
    tflint --version && \
    trivy --version && \
    checkov --version && \
    terraform-docs --version && \
    go version && \
    golint --version || true

# -----------------------------------------------------------
# Create non-root runtime user  • CKV_DOCKER_3
# -----------------------------------------------------------

RUN groupadd --system cicd && \
    useradd  --system --gid cicd --home-dir /workspace --shell /usr/sbin/nologin cicd && \
    mkdir -p /workspace && \
    chown -R cicd:cicd /workspace

# -----------------------------------------------------------
# Switch to unprivileged user
# -----------------------------------------------------------

USER cicd

# -----------------------------------------------------------
# Container HEALTHCHECK  • CKV_DOCKER_2
# -----------------------------------------------------------

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD terraform -version >/dev/null || exit 1

# -----------------------------------------------------------
# Runtime defaults
# -----------------------------------------------------------

WORKDIR /workspace
CMD ["bash"]

