FROM golang:1.24.4-bookworm AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -p 2 -trimpath -o /out/ai-brain ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /application
COPY --from=build /out/ai-brain /application/bin/ai-brain
ENV TZ=Asia/Shanghai
ENTRYPOINT ["/application/bin/ai-brain"]
