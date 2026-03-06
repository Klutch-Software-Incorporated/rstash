FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X rstash/internal/config.Version=${VERSION}" -o /rstash .

FROM gcr.io/distroless/static-debian12
COPY --from=build /rstash /rstash
EXPOSE 8080
ENTRYPOINT ["/rstash"]
