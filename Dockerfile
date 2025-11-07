FROM golang:1.24.7 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o main cmd/segmentation/main.go

FROM node:18-slim
WORKDIR /app
COPY --from=builder /app/main .
COPY .env .
RUN npm install -g pm2
EXPOSE 8083
CMD ["pm2-runtime", "start", "./main", "--name", "Nexora Segmentation Service"]