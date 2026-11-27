# Image of the Killy server. buildkitd pulls docker.io images through the cluster's mirror.
FROM --platform=$BUILDPLATFORM golang:1.27.4@sha256:5f1b98fa02fe396ebce2a0227a0e08309e11792915b9dd2e1f71053d7e7e6666 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG TARGETARCH
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath \
	-ldflags "-X github.com/sumiredc/killy/internal/version.Version=${VERSION}" \
	-o /out/killy ./cmd/killy

FROM scratch
# The server sends pushes to Expo over HTTPS; without these roots every push fails.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/killy /killy
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/killy", "server"]
