#!/bin/bash

# 清理所有调试进程的脚本
# 用于强制终止无法正常停止的调试会话

echo "🔍 正在查找调试进程..."

# 查找所有调试相关进程
DEBUG_PIDS=$(ps aux | grep -E "(dlv dap|__debug_bin|debugserver.*genioAI_server)" | grep -v grep | awk '{print $2}')

if [ -z "$DEBUG_PIDS" ]; then
    echo "✅ 未找到运行中的调试进程"
    exit 0
fi

echo "📋 找到以下调试进程:"
ps aux | grep -E "(dlv dap|__debug_bin|debugserver.*genioAI_server)" | grep -v grep

echo ""
echo "🛑 正在终止调试进程..."

# 终止所有调试进程
for PID in $DEBUG_PIDS; do
    echo "  - 终止进程 $PID"
    kill -9 $PID 2>/dev/null
done

# 等待进程终止
sleep 1

# 验证是否清理成功
REMAINING=$(ps aux | grep -E "(dlv dap|__debug_bin|debugserver.*genioAI_server)" | grep -v grep | wc -l)

if [ "$REMAINING" -eq 0 ]; then
    echo "✅ 所有调试进程已成功清理"
else
    echo "⚠️  仍有 $REMAINING 个进程未能终止，请手动检查"
fi
