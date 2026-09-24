FROM public.ecr.aws/docker/library/golang:1.24 AS build
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -p 2 -trimpath -o /out/ai-brain ./cmd/server

FROM public.ecr.aws/docker/library/debian:trixie-slim
RUN sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list.d/debian.sources \
    && apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /application
COPY --from=build /out/ai-brain /application/bin/ai-brain
ENV TZ=Asia/Shanghai
ENTRYPOINT ["/application/bin/ai-brain"]
