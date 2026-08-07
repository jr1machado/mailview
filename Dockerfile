FROM alpine:3.23.3

# Install dependencies
RUN apk --no-cache add ca-certificates tzdata \
    && addgroup -S -g 10001 mailview \
    && adduser -S -D -H -u 10001 -G mailview -s /sbin/nologin mailview

# Set the working directory
WORKDIR /listmonk

# Copy only the necessary files
COPY --chown=mailview:mailview listmonk .
COPY --chown=mailview:mailview config.toml.sample config.toml

# Copy the entrypoint script
COPY --chown=mailview:mailview docker-entrypoint.sh /usr/local/bin/

# Make the entrypoint script executable
RUN chmod 0555 /usr/local/bin/docker-entrypoint.sh \
    && mkdir -p /listmonk/uploads \
    && chown -R mailview:mailview /listmonk

USER 10001:10001

# Expose the application port
EXPOSE 9000

# Set the entrypoint
ENTRYPOINT ["docker-entrypoint.sh"]

# Define the command to run the application
CMD ["./listmonk"]
