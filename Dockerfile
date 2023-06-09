FROM golang:1.19 AS build

WORKDIR /app

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build driver.go

FROM scratch
WORKDIR /app
COPY --from=build /app/driver  /app/.env /app/drivers.json ./

ENTRYPOINT [ "./driver" ]