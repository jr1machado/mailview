# syntax=docker/dockerfile:1

# --- frontend: builds the Vue admin (frontend/dist) and the email-builder
# bundle it embeds (frontend/public/static/email-builder). ---
FROM node:22-alpine AS frontend
WORKDIR /src

# frontend's postinstall (altcha) writes into ../static/public/static, so that
# directory has to exist before `yarn install` runs.
COPY static ./static
COPY frontend/package.json frontend/yarn.lock ./frontend/
RUN --mount=type=cache,target=/usr/local/share/.cache/yarn \
    cd frontend && yarn install --frozen-lockfile

COPY frontend/email-builder/package.json frontend/email-builder/yarn.lock ./frontend/email-builder/
RUN --mount=type=cache,target=/usr/local/share/.cache/yarn \
    cd frontend/email-builder && yarn install --frozen-lockfile

COPY frontend ./frontend
RUN cd frontend/email-builder && yarn build \
    && mkdir -p ../public/static/email-builder \
    && cp -r dist/* ../public/static/email-builder/
RUN cd frontend && yarn build

# --- backend: compiles the Go binary and packs every static asset
# (frontend/dist, SQL, i18n, ...) into it with stuffbin, same as `make dist`. ---
FROM golang:1.26.5 AS backend
WORKDIR /src

RUN go install github.com/knadh/stuffbin/...@latest

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend /src/frontend/dist ./frontend/dist
COPY --from=frontend /src/frontend/public/static/email-builder ./static/public/static/email-builder

ARG MAILVIEW_VERSION=v0.6.0
ARG MAILVIEW_COMMIT=unknown
ARG MAILVIEW_BUILD_DATE=unknown
RUN CGO_ENABLED=0 go build -o MailView \
      -ldflags="-s -w -X 'main.buildString=${MAILVIEW_VERSION} (#${MAILVIEW_COMMIT} ${MAILVIEW_BUILD_DATE})' -X 'main.versionString=${MAILVIEW_VERSION}'" \
      ./cmd \
    && stuffbin -a stuff -in MailView -out MailView \
      config.toml.sample schema.sql queries:/queries permissions.json \
      static/public:/public static/email-templates frontend/dist:/admin i18n:/i18n

# --- final: just the packed binary and entrypoint on a minimal base. ---
FROM alpine:3.23.3
RUN apk --no-cache add ca-certificates tzdata \
    && addgroup -S -g 10001 mailview \
    && adduser -S -D -H -u 10001 -G mailview -s /sbin/nologin mailview

WORKDIR /mailview
COPY --from=backend --chown=mailview:mailview /src/MailView .
COPY --chown=mailview:mailview config.toml.sample config.toml
COPY --chown=mailview:mailview docker-entrypoint.sh /usr/local/bin/

RUN chmod 0555 /usr/local/bin/docker-entrypoint.sh \
    && mkdir -p /mailview/uploads /mailview/imports \
    && chown -R mailview:mailview /mailview

USER 10001:10001
EXPOSE 9000
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["./MailView"]
