FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/ocpi-simulator ./cmd/ocpi-simulator

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /out/ocpi-simulator /ocpi-simulator
USER nonroot:nonroot
EXPOSE 8081
ENTRYPOINT ["/ocpi-simulator"]
