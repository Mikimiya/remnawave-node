#!/bin/bash

# ============================================
# Node Exporter 卸载脚本
# ============================================

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

echo -e "${RED}"
echo "============================================"
echo "   Node Exporter 卸载脚本"
echo "============================================"
echo -e "${NC}"

if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}❌ 请使用 root 用户运行${NC}"
    exit 1
fi

read -p "确定要卸载 Node Exporter? (y/n): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    exit 0
fi

echo "[1/4] 停止服务..."
systemctl stop node_exporter 2>/dev/null || true

echo "[2/4] 禁用服务..."
systemctl disable node_exporter 2>/dev/null || true

echo "[3/4] 删除文件..."
rm -f /etc/systemd/system/node_exporter.service
rm -f /usr/local/bin/node_exporter

echo "[4/4] 删除用户..."
userdel node_exporter 2>/dev/null || true

systemctl daemon-reload

echo ""
echo -e "${GREEN}✅ Node Exporter 已卸载${NC}"
