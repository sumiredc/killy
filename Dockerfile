# Image of the Killy server. buildkitd pulls docker.io images through the cluster's mirror.
FROM --platform=$BUILDPLATFORM golang:1.27.3@sha256:e519d3b0a5b285e5aa53ab9eef02bc6bbb9cb04608389b743a136a9ce75f0d1e AS build
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
