FROM public.ecr.aws/docker/library/golang:1.24 AS build
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
ENV GOMAXPROCS=2
WORKDIR /src
COPY go.mod go.sum ./
COPY pkg ./pkg
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY conf/*.go conf/model_config.json ./conf/
COPY assets/nutrition_reference.json ./assets/
COPY assets/migrations ./assets/migrations
RUN CGO_ENABLED=1 go build -p 1 -trimpath -o /out/backend ./cmd

FROM public.ecr.aws/docker/library/debian:trixie-slim
RUN sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list.d/debian.sources \
    && apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /application
COPY --from=build /out/backend /application/bin/backend
COPY --from=build /src/conf/model_config.json /application/conf/model_config.json
COPY --from=build /src/assets /application/assets
ENV TZ=Asia/Shanghai
ENTRYPOINT ["/application/bin/backend"]
