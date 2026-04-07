# =============================================
# Build arguments
# =============================================
ARG UBUNTU_VERSION=focal
ARG GO_VERSION=1.23.0

# =============================================
FROM ubuntu:${UBUNTU_VERSION} AS base

ARG GO_VERSION

ENV DEBIAN_FRONTEND=noninteractive

# Install core utilities and the PostgreSQL client
RUN apt-get update && apt-get install -y \
    apt-utils \
    bash-completion \
    curl \
    git \
    nano \
    postgresql-client \
    sudo \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Install Go
RUN wget https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz && \
    rm go${GO_VERSION}.linux-amd64.tar.gz

# Add Go to the system PATH
ENV PATH="/usr/local/go/bin:${PATH}"

# =============================================
FROM base AS with_user

ARG USERNAME
RUN : "${USERNAME:?Build argument USERNAME must be provided and not empty}"

# Create the user (adapted for Ubuntu 20.04 where 'ubuntu' user doesn't exist by default)
RUN if id -u ${USERNAME} >/dev/null 2>&1; then \
        echo "User ${USERNAME} already exists."; \
    else \
        useradd -m -s /bin/bash ${USERNAME}; \
    fi

# Grant passwordless sudo access
RUN echo "${USERNAME} ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/${USERNAME} \
    && chmod 0440 /etc/sudoers.d/${USERNAME}