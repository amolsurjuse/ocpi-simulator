FROM node:22-alpine AS ui-build
WORKDIR /ui
COPY ui/package.json ui/package-lock.json ./
RUN npm ci
COPY ui/ ./
RUN npm run build -- --configuration production --base-href /

FROM golang:1.22-alpine AS go-build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/ocpi-simulator ./cmd/ocpi-simulator

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=go-build /out/ocpi-simulator /ocpi-simulator
COPY --from=ui-build /ui/dist/ocpi-simulator-ui/browser /ui
ENV UI_STATIC_DIR=/ui
ENV UI_ENABLED=true
USER nonroot:nonroot
EXPOSE 8081
ENTRYPOINT ["/ocpi-simulator"]
