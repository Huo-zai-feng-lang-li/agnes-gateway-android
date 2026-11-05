#!/data/data/com.termux/files/usr/bin/bash

# Termux 一键运行脚本
# 赋予权限并启动
chmod +x ./agnes-gateway-arm64
echo "=========================================="
echo "  Agnes 手机反代网关 (Termux 极速模式)"
echo "=========================================="
echo "提示: 请确保已传入你的 Agnes Key 列表"
echo "用法示例: ./agnes-gateway-arm64 --port=8080 --keys=\"sk-key1,sk-key2\""
echo "=========================================="
./agnes-gateway-arm64 --port=8080 "$@"
