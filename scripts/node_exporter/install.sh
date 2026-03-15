#!/bin/bash

# ============================================
# Node Exporter 一键安装脚本
# 版本: 1.10.2
# 支持: Ubuntu/Debian/CentOS/RHEL
# ============================================

set -e

VERSION="1.10.2"
ARCH="linux-amd64"
DOWNLOAD_URL="https://github.com/prometheus/node_exporter/releases/download/v${VERSION}/node_exporter-${VERSION}.${ARCH}.tar.gz"
INSTALL_DIR="/usr/local/bin"
SERVICE_FILE="/etc/systemd/system/node_exporter.service"

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}"
echo "============================================"
echo "   Node Exporter 一键安装脚本"
echo "   版本: v${VERSION}"
echo "============================================"
echo -e "${NC}"

# 检查 root 权限
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}❌ 请使用 root 用户运行此脚本${NC}"
    echo "运行: sudo bash $0"
    exit 1
fi

# 检查是否已安装
if systemctl is-active --quiet node_exporter 2>/dev/null; then
    echo -e "${YELLOW}⚠️  Node Exporter 已在运行${NC}"
    read -p "是否重新安装? (y/n): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 0
    fi
    systemctl stop node_exporter
fi

echo -e "${GREEN}[1/6]${NC} 创建系统用户..."
useradd -rs /bin/false node_exporter 2>/dev/null || true

echo -e "${GREEN}[2/6]${NC} 下载 Node Exporter v${VERSION}..."
cd /tmp
if command -v wget &> /dev/null; then
    wget -q --show-progress ${DOWNLOAD_URL}
elif command -v curl &> /dev/null; then
    curl -LO ${DOWNLOAD_URL}
else
    echo -e "${RED}❌ 需要 wget 或 curl${NC}"
    exit 1
fi

echo -e "${GREEN}[3/6]${NC} 解压安装..."
tar xzf node_exporter-${VERSION}.${ARCH}.tar.gz
mv node_exporter-${VERSION}.${ARCH}/node_exporter ${INSTALL_DIR}/
chown node_exporter:node_exporter ${INSTALL_DIR}/node_exporter
chmod +x ${INSTALL_DIR}/node_exporter

echo -e "${GREEN}[4/6]${NC} 创建 systemd 服务..."
cat > ${SERVICE_FILE} << 'EOF'
[Unit]
Description=Node Exporter
Documentation=https://prometheus.io/docs/guides/node-exporter/
Wants=network-online.target
After=network-online.target

[Service]
User=node_exporter
Group=node_exporter
Type=simple
Restart=on-failure
RestartSec=5s
ExecStart=/usr/local/bin/node_exporter

[Install]
WantedBy=multi-user.target
EOF

echo -e "${GREEN}[5/6]${NC} 启动服务..."
systemctl daemon-reload
systemctl enable node_exporter
systemctl start node_exporter

echo -e "${GREEN}[6/6]${NC} 清理临时文件..."
rm -rf /tmp/node_exporter-${VERSION}*

# 获取 IP
IP=$(hostname -I | awk '{print $1}')

echo ""
echo -e "${GREEN}============================================${NC}"
echo -e "${GREEN}   ✅ 安装完成!${NC}"
echo -e "${GREEN}============================================${NC}"
echo ""
echo -e "服务状态: ${GREEN}$(systemctl is-active node_exporter)${NC}"
echo ""
echo -e "📊 访问地址: ${YELLOW}http://${IP}:9100/metrics${NC}"
echo ""
echo "常用命令:"
echo "  查看状态: systemctl status node_exporter"
echo "  查看日志: journalctl -u node_exporter -f"
echo "  重启服务: systemctl restart node_exporter"
echo "  停止服务: systemctl stop node_exporter"
echo ""
echo "Prometheus 配置:"
echo "  scrape_configs:"
echo "    - job_name: 'node-exporter'"
echo "      static_configs:"
echo "        - targets: ['${IP}:9100']"
echo ""
