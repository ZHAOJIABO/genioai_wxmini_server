#!/bin/bash

################################################################################
# GenioAI Backend 滚动更新部署脚本
# 用途：使用滚动更新策略，将停机时间缩短到最小（约5-10秒）
# 策略：先构建镜像 → 快速启动新容器 → 立即停止旧容器
################################################################################

set -e  # 遇到错误立即退出

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 项目配置
PROJECT_DIR="/opt/genio-backend"
GIT_BRANCH="main"
BACKUP_DIR="/opt/genio-backend-backups"

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查是否在项目目录
check_directory() {
    if [ ! -f "docker-compose.yml" ]; then
        log_error "未找到 docker-compose.yml，请在项目根目录执行此脚本"
        exit 1
    fi

    # 检查 .env 文件
    if [ ! -f ".env" ]; then
        log_error "未找到 .env 文件，请先创建并配置环境变量"
        exit 1
    fi
}

# 检查 Git 仓库状态
check_git_status() {
    log_info "检查 Git 仓库状态..."

    # 检查 genioAI_server 子目录是否是 Git 仓库
    if [ -d "genioAI_server/.git" ]; then
        cd genioAI_server

        # 检查是否有未提交的更改
        if ! git diff-index --quiet HEAD -- 2>/dev/null; then
            log_warning "检测到未提交的更改"
            cd ..
            read -p "是否继续部署？(y/n): " -n 1 -r
            echo
            if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                log_info "部署已取消"
                exit 0
            fi
            return 0
        fi

        cd ..
    elif [ -d ".git" ]; then
        # 如果当前目录是 Git 仓库
        if ! git diff-index --quiet HEAD -- 2>/dev/null; then
            log_warning "检测到未提交的更改"
            read -p "是否继续部署？(y/n): " -n 1 -r
            echo
            if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                log_info "部署已取消"
                exit 0
            fi
        fi
    else
        log_warning "未找到 Git 仓库，跳过代码拉取"
        return 0
    fi
}

# 拉取最新代码
pull_latest_code() {
    local GIT_DIR=""

    # 检查 genioAI_server 子目录是否是 Git 仓库
    if [ -d "genioAI_server/.git" ]; then
        GIT_DIR="genioAI_server"
        log_info "检测到 Git 仓库: genioAI_server/"
    elif [ -d ".git" ]; then
        GIT_DIR="."
        log_info "检测到 Git 仓库: 当前目录"
    else
        return 0
    fi

    log_info "拉取最新代码..."

    cd "$GIT_DIR"

    git fetch origin

    LOCAL=$(git rev-parse @ 2>/dev/null || echo "")
    REMOTE=$(git rev-parse @{u} 2>/dev/null || echo "")

    if [ -z "$LOCAL" ] || [ -z "$REMOTE" ]; then
        log_warning "无法检查 Git 版本，跳过拉取"
        cd - > /dev/null
        return 0
    fi

    if [ "$LOCAL" = "$REMOTE" ]; then
        log_info "代码已是最新版本"
    else
        log_info "发现新版本，开始拉取..."
        git pull origin "$GIT_BRANCH"
        log_success "代码更新完成"
    fi

    # 显示最新提交
    log_info "最新提交："
    git log -1 --pretty=format:"%h - %an, %ar : %s" | cat
    echo
    echo

    cd - > /dev/null
}

# 创建备份
create_backup() {
    log_info "创建当前镜像备份..."

    TIMESTAMP=$(date +%Y%m%d_%H%M%S)
    mkdir -p "$BACKUP_DIR"

    # 导出当前镜像
    if docker images | grep -q "genio-backend_backend"; then
        docker save genio-backend_backend:latest | gzip > "$BACKUP_DIR/genio-backend_$TIMESTAMP.tar.gz" &
        SAVE_PID=$!

        # 显示进度
        while kill -0 $SAVE_PID 2>/dev/null; do
            echo -n "."
            sleep 1
        done
        echo

        log_success "备份已保存到: $BACKUP_DIR/genio-backend_$TIMESTAMP.tar.gz"

        # 只保留最近 3 个备份
        cd "$BACKUP_DIR"
        ls -t genio-backend_*.tar.gz 2>/dev/null | tail -n +4 | xargs -r rm
        cd - > /dev/null
    else
        log_warning "未找到现有镜像，跳过备份"
    fi
}

# 构建新镜像（服务继续运行）
build_image() {
    log_info "开始构建新的 Docker 镜像（旧容器继续运行）..."

    docker-compose build --no-cache backend

    log_success "镜像构建完成"
}

# 确保数据库和 Redis 运行
ensure_dependencies() {
    log_info "确保依赖服务运行中..."

    # 创建共享网络（如果不存在）
    if ! docker network ls | grep -q genio-shared; then
        log_info "创建共享网络 genio-shared..."
        docker network create genio-shared
    fi

    # 启动 MySQL
    if ! docker ps | grep -q "genio-backend-mysql.*healthy"; then
        log_info "启动 MySQL..."
        docker-compose up -d backend-mysql

        # 等待 MySQL 健康
        log_info "等待 MySQL 就绪..."
        RETRY=0
        while [ $RETRY -lt 30 ]; do
            if docker ps | grep -q "genio-backend-mysql.*healthy"; then
                log_success "MySQL 已就绪"
                break
            fi
            echo -n "."
            sleep 2
            RETRY=$((RETRY + 1))
        done
        echo
    else
        log_success "MySQL 已在运行"
    fi

    # 启动 Redis
    if ! docker ps | grep -q "genio-backend-redis.*healthy"; then
        log_info "启动 Redis..."
        docker-compose up -d backend-redis

        # 等待 Redis 健康
        log_info "等待 Redis 就绪..."
        RETRY=0
        while [ $RETRY -lt 15 ]; do
            if docker ps | grep -q "genio-backend-redis.*healthy"; then
                log_success "Redis 已就绪"
                break
            fi
            echo -n "."
            sleep 2
            RETRY=$((RETRY + 1))
        done
        echo
    else
        log_success "Redis 已在运行"
    fi
}

# 滚动更新：快速切换容器
rolling_update() {
    log_info "开始滚动更新（最小停机时间）..."
    echo

    # 记录开始时间
    START_TIME=$(date +%s)

    # 停止并删除旧容器
    if docker ps | grep -q "genio-backend"; then
        log_info "⏸  停止旧容器..."
        docker-compose stop backend
        docker-compose rm -f backend
    fi

    # 立即启动新容器
    log_info "🚀 启动新容器..."
    docker-compose up -d backend

    # 计算停机时间
    END_TIME=$(date +%s)
    DOWNTIME=$((END_TIME - START_TIME))

    log_success "容器切换完成（停机时间：${DOWNTIME}秒）"
}

# 等待服务健康检查
wait_for_health() {
    log_info "等待服务健康检查..."

    RETRY_COUNT=0
    MAX_RETRIES=30

    while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
        if docker ps | grep -q "genio-backend.*healthy"; then
            echo
            log_success "服务健康检查通过"
            return 0
        fi

        RETRY_COUNT=$((RETRY_COUNT + 1))
        echo -n "."
        sleep 2
    done

    echo
    log_error "服务健康检查失败，请查看日志"
    docker-compose logs --tail=50 backend
    return 1
}

# 显示服务状态
show_status() {
    log_info "当前服务状态："
    echo
    docker-compose ps
    echo

    log_info "最近日志："
    docker-compose logs --tail=20 backend
}

# 测试服务
test_service() {
    log_info "测试服务连接..."

    # 测试 Metrics 端口
    if curl -f -s http://localhost:9188/metrics > /dev/null 2>&1; then
        log_success "Metrics 端点正常 (9188)"
    else
        log_warning "Metrics 端点可能未就绪"
    fi

    # 测试 HTTP 端口
    if curl -f -s http://localhost:8200 > /dev/null 2>&1; then
        log_success "HTTP 服务正常 (8200)"
    else
        log_warning "HTTP 服务响应异常（可能根路由未定义）"
    fi

    # 测试 gRPC 端口
    if command -v nc >/dev/null 2>&1; then
        if nc -z localhost 8181 2>/dev/null; then
            log_success "gRPC 服务正常 (8181)"
        else
            log_warning "gRPC 服务可能未就绪"
        fi
    fi
}

# 清理旧镜像
cleanup_old_images() {
    log_info "清理未使用的镜像..."

    docker image prune -f > /dev/null 2>&1

    log_success "清理完成"
}

# 回滚函数
rollback() {
    log_warning "开始回滚到上一个版本..."

    LATEST_BACKUP=$(ls -t "$BACKUP_DIR"/genio-backend_*.tar.gz 2>/dev/null | head -1)

    if [ -z "$LATEST_BACKUP" ]; then
        log_error "未找到备份文件，无法回滚"
        exit 1
    fi

    log_info "加载备份镜像: $LATEST_BACKUP"
    docker load < "$LATEST_BACKUP"

    log_info "重启容器..."
    docker-compose stop backend
    docker-compose rm -f backend
    docker-compose up -d backend

    log_success "回滚完成"
}

# 主流程
main() {
    echo "=========================================="
    echo "  GenioAI Backend 滚动更新部署脚本"
    echo "  (Rolling Update - 最小停机时间)"
    echo "=========================================="
    echo

    # 检查环境
    check_directory
    check_git_status

    # 拉取代码
    pull_latest_code

    # 确认部署
    echo
    read -p "是否继续部署？(y/n): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "部署已取消"
        exit 0
    fi

    # 开始部署
    echo
    log_info "开始部署流程..."
    echo

    # 创建日志目录
    mkdir -p logs

    # 确保依赖服务运行
    ensure_dependencies

    # 创建备份
    create_backup

    # 构建镜像（旧容器继续运行）
    build_image

    # 滚动更新：快速切换
    rolling_update

    # 等待健康检查
    if wait_for_health; then
        # 测试服务
        test_service

        # 显示状态
        show_status

        # 清理旧镜像
        cleanup_old_images

        echo
        echo "=========================================="
        log_success "部署成功完成！"
        echo "=========================================="
        echo
        log_info "访问地址："
        echo "  - API (外部): https://genioai.appbobo.com"
        echo "  - API (本地): http://localhost:8200"
        echo "  - gRPC:       localhost:8181"
        echo "  - Metrics:    http://localhost:9188/metrics"
        echo
        log_info "查看日志: docker-compose logs -f backend"
        log_info "查看状态: docker-compose ps"
        log_info "回滚命令: ./deploy-zero-downtime.sh --rollback"
        echo
    else
        log_error "部署失败"
        echo
        read -p "是否回滚到上一个版本？(y/n): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            rollback
        fi
        exit 1
    fi
}

# 处理脚本参数
case "${1:-}" in
    --rollback)
        rollback
        ;;
    --status)
        show_status
        ;;
    --logs)
        docker-compose logs -f backend
        ;;
    --help)
        echo "GenioAI Backend 滚动更新部署脚本"
        echo
        echo "用法:"
        echo "  ./deploy-zero-downtime.sh           - 执行滚动更新部署"
        echo "  ./deploy-zero-downtime.sh --status  - 查看服务状态"
        echo "  ./deploy-zero-downtime.sh --logs    - 查看实时日志"
        echo "  ./deploy-zero-downtime.sh --rollback - 回滚到上一个版本"
        echo "  ./deploy-zero-downtime.sh --help    - 显示帮助信息"
        echo
        echo "部署策略："
        echo "  使用滚动更新（Rolling Update）实现最小停机"
        echo "  1. 先构建新镜像（旧容器继续运行）"
        echo "  2. 快速停止旧容器"
        echo "  3. 立即启动新容器"
        echo "  4. 停机时间约 5-10 秒"
        ;;
    *)
        main
        ;;
esac
