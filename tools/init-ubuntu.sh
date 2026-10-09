#!/bin/bash
# WSL Ubuntu-24.04 初始化（以 root 执行）
set -e
echo dev:dev123 | chpasswd
echo 'dev ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/dev
chmod 440 /etc/sudoers.d/dev
printf '[user]\ndefault=dev\n[boot]\nsystemd=true\n' > /etc/wsl.conf
mkdir -p /etc/apt/sources.list.d
# 国内源加速（清华 TUNA，24.04 为 deb822 格式）
cat > /etc/apt/sources.list.d/tuna.sources <<'EOF'
Types: deb
URIs: https://mirrors.tuna.tsinghua.edu.cn/ubuntu
Suites: noble noble-updates noble-security
Components: main restricted universe multiverse
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg
EOF
rm -f /etc/apt/sources.list.d/ubuntu.sources
echo '=== 初始化完成 ==='
cat /etc/wsl.conf
ls /etc/sudoers.d/
