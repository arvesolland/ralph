# Ralph development environment
# Go toolchain for building and testing ralph

FROM 113590434675.dkr.ecr.ap-southeast-2.amazonaws.com/ralph-fleet-worker:latest

USER root

# Install Go 1.22
ARG GO_VERSION=1.22.5
RUN ARCH=$(dpkg --print-architecture) && \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz" | tar -C /usr/local -xz && \
    rm -rf /var/lib/apt/lists/*

ENV PATH="/usr/local/go/bin:/home/agent/go/bin:${PATH}"
ENV GOPATH="/home/agent/go"

# golangci-lint for linting
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b /usr/local/bin

USER agent

# Verify installations
RUN go version && golangci-lint --version
