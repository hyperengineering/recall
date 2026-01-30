FROM alpine:3.19

# Install CA certificates for HTTPS connections to Engram
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -g '' recall
USER recall

# Copy binary
COPY recall /usr/local/bin/recall

# Set entrypoint
ENTRYPOINT ["recall"]
