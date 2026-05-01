FROM mcr.microsoft.com/devcontainers/go:1.25

# Optional quality-of-life packages for local development.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      ca-certificates \
      curl \
      git && \
    rm -rf /var/lib/apt/lists/*
