FROM golang:1.22 AS build-stage

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /topologycalculator

FROM gcr.io/distroless/static AS production

WORKDIR /

COPY --from=build-stage /topologycalculator /topologycalculator

USER nonroot:nonroot

CMD ["/topologycalculator"]