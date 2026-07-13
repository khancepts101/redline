FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG COMMAND=demo-service
RUN CGO_ENABLED=0 go build -trimpath -o /out/app ./cmd/${COMMAND}
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
ENTRYPOINT ["/app"]
