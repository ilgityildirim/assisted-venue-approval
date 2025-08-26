# Development Dockerfile for Go application
FROM golang:1.25-alpine

# Install dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Ensure Go can manage toolchains if needed
ENV TZ=UTC \
    GOTOOLCHAIN=auto

# Expose ports
EXPOSE 8080 8081