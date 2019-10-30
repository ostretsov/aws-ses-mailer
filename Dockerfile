################################
# STEP 1 build executable binary
FROM golang:1.13.3-alpine as builder

# Install git + SSL ca certificates
# Git is required for fetching the dependencies
# Ca-certificates is required to call HTTPS endpoints
RUN apk update && apk add --no-cache git ca-certificates tzdata curl && update-ca-certificates

# Create appuser
RUN adduser -D -g '' appuser

WORKDIR $GOPATH/src/aws-ses-mailer
COPY . .

# Fetch dependencies
RUN go mod download

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -a -installsuffix cgo -o /go/bin/aws-ses-mailer .

############################
# STEP 2 build a small image
FROM scratch

# Import from builder
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd

# Copy our static executable
COPY --from=builder /go/bin/aws-ses-mailer /go/bin/aws-ses-mailer

# Use an unprivileged user
USER appuser

ENTRYPOINT ["/go/bin/aws-ses-mailer"]