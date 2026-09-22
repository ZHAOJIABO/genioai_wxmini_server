#!/bin/bash

# VA Vision AI Server - Protocol Buffer 代码生成脚本
# 用途：从 pkg/va_interface 生成 protobuf 代码到 internal/va_interface

set -e  # 遇到错误立即退出

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 项目根目录（脚本所在目录的上一级）
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# 路径配置
PROTO_DIR="$PROJECT_ROOT/pkg/va_interface"
OUTPUT_DIR="$PROJECT_ROOT/internal/va_interface"
THIRD_PARTY_DIR="$PROTO_DIR/third_party/googleapis"

# 确保在项目根目录执行
cd "$PROJECT_ROOT"

# 打印带颜色的消息
print_info() {
    echo -e "${BLUE}ℹ ${NC}$1"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

# 检查必要的工具
check_tools() {
    print_info "检查必要的工具..."

    local missing_tools=()

    if ! command -v protoc &> /dev/null; then
        missing_tools+=("protoc")
    fi

    # 添加 $GOPATH/bin 到 PATH
    export PATH="$PATH:$HOME/go/bin"

    if ! command -v protoc-gen-go &> /dev/null; then
        missing_tools+=("protoc-gen-go")
    fi

    if ! command -v protoc-gen-go-grpc &> /dev/null; then
        missing_tools+=("protoc-gen-go-grpc")
    fi

    if ! command -v protoc-gen-grpc-gateway &> /dev/null; then
        missing_tools+=("protoc-gen-grpc-gateway")
    fi

    if [ ${#missing_tools[@]} -ne 0 ]; then
        print_error "缺少以下工具: ${missing_tools[*]}"
        echo ""
        echo "请安装缺少的工具："
        echo "  brew install protobuf                                                    # 安装 protoc"
        echo "  go install google.golang.org/protobuf/cmd/protoc-gen-go@latest          # 安装 protoc-gen-go"
        echo "  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest         # 安装 protoc-gen-go-grpc"
        echo "  go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest  # 安装 protoc-gen-grpc-gateway"
        exit 1
    fi

    print_success "所有必要的工具都已安装"
}

# 检查目录结构
check_directories() {
    print_info "检查目录结构..."

    if [ ! -d "$PROTO_DIR" ]; then
        print_error "Proto 目录不存在: $PROTO_DIR"
        exit 1
    fi

    if [ ! -d "$THIRD_PARTY_DIR" ]; then
        print_error "Third party 目录不存在: $THIRD_PARTY_DIR"
        exit 1
    fi

    # 确保输出目录存在
    mkdir -p "$OUTPUT_DIR"

    print_success "目录结构检查通过"
}

# 清理旧的生成文件
clean_generated_files() {
    if [ "$1" == "--clean" ]; then
        print_info "清理旧的生成文件..."
        find "$OUTPUT_DIR" -name "*.pb.go" -delete
        find "$OUTPUT_DIR" -name "*.pb.gw.go" -delete
        print_success "清理完成"
    fi
}

# 生成 protobuf 代码
generate_proto() {
    print_info "开始生成 protobuf 代码..."
    echo ""

    local success_count=0
    local fail_count=0
    local total_count=0

    # 遍历所有 proto 文件
    for proto_file in "$PROTO_DIR"/*.proto; do
        if [ -f "$proto_file" ]; then
            total_count=$((total_count + 1))
            local basename=$(basename "$proto_file")

            echo -n "  生成 $basename ... "

            # 将 protoc 的输出重定向到临时文件
            local tmp_output=$(mktemp)
            if protoc \
                --proto_path="$PROTO_DIR" \
                --proto_path="$THIRD_PARTY_DIR" \
                --go_out="$OUTPUT_DIR" \
                --go_opt=paths=source_relative \
                --go-grpc_out="$OUTPUT_DIR" \
                --go-grpc_opt=paths=source_relative \
                --grpc-gateway_out="$OUTPUT_DIR" \
                --grpc-gateway_opt=paths=source_relative \
                --grpc-gateway_opt=generate_unbound_methods=true \
                "$proto_file" 2>&1 > "$tmp_output"; then
                echo -e "${GREEN}✓${NC}"
                success_count=$((success_count + 1))
            else
                echo -e "${RED}✗${NC}"
                # 显示错误信息（排除 WARNING）
                grep -v "WARNING" "$tmp_output" | head -5
                fail_count=$((fail_count + 1))
            fi
            rm -f "$tmp_output"
        fi
    done

    echo ""
    if [ $fail_count -gt 0 ]; then
        print_warning "代码生成完成（有错误）"
    else
        print_success "代码生成完成！"
    fi
    echo "  总计: $total_count 个文件"
    echo "  成功: $success_count 个文件"
    if [ $fail_count -gt 0 ]; then
        echo "  失败: $fail_count 个文件"
    fi
}

# 统计生成的文件
show_statistics() {
    print_info "生成文件统计："

    local pb_count=$(find "$OUTPUT_DIR" -name "*.pb.go" ! -name "*_grpc.pb.go" ! -name "*.pb.gw.go" | wc -l | xargs)
    local grpc_count=$(find "$OUTPUT_DIR" -name "*_grpc.pb.go" | wc -l | xargs)
    local gw_count=$(find "$OUTPUT_DIR" -name "*.pb.gw.go" | wc -l | xargs)
    local total_count=$((pb_count + grpc_count + gw_count))

    echo "  .pb.go 文件 (消息定义):     $pb_count"
    echo "  _grpc.pb.go 文件 (gRPC):    $grpc_count"
    echo "  .pb.gw.go 文件 (HTTP网关):  $gw_count"
    echo "  ─────────────────────────────────"
    echo "  总计:                        $total_count"
}

# 验证编译
verify_build() {
    if [ "$1" == "--verify" ] || [ "$1" == "-v" ]; then
        print_info "验证 Go 代码编译..."

        if go build -o /tmp/va_server_test ./cmd 2>&1 | grep -v "webp" | grep -E "(error|Error)"; then
            print_error "编译失败"
            rm -f /tmp/va_server_test
            exit 1
        else
            print_success "编译验证通过"
            rm -f /tmp/va_server_test
        fi
    fi
}

# 显示帮助信息
show_help() {
    cat << EOF
用法: $0 [选项]

选项:
  --clean       生成前清理旧的 .pb.go 文件
  --verify, -v  生成后验证 Go 代码编译
  --help, -h    显示此帮助信息

示例:
  $0                  # 直接生成代码
  $0 --clean          # 清理后生成
  $0 --clean --verify # 清理、生成、验证
  $0 -v               # 生成并验证

生成路径:
  输入: $PROTO_DIR
  输出: $OUTPUT_DIR
EOF
}

# 主函数
main() {
    # 解析参数
    local do_clean=false
    local do_verify=false

    for arg in "$@"; do
        case $arg in
            --clean)
                do_clean=true
                ;;
            --verify|-v)
                do_verify=true
                ;;
            --help|-h)
                show_help
                exit 0
                ;;
            *)
                print_error "未知参数: $arg"
                echo "使用 --help 查看帮助信息"
                exit 1
                ;;
        esac
    done

    # 打印标题
    echo ""
    echo "════════════════════════════════════════════════════════"
    echo "  VA Vision AI Server - Protocol Buffer 代码生成"
    echo "════════════════════════════════════════════════════════"
    echo ""

    # 执行流程
    check_tools
    check_directories

    if [ "$do_clean" = true ]; then
        clean_generated_files --clean
    fi

    generate_proto
    show_statistics

    if [ "$do_verify" = true ]; then
        echo ""
        verify_build --verify
    fi

    echo ""
    print_success "所有操作完成！"
    echo ""
}

# 执行主函数
main "$@"
