#!/usr/bin/env bash
set -e

# 获取脚本所在目录路径（确保在项目根目录下运行）
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

VERSION_FILE="VERSION"

if [ ! -f "$VERSION_FILE" ]; then
    echo "❌ 错误: 未找到 $VERSION_FILE 文件！"
    exit 1
fi

VERSION=$(tr -d '\r\n' < "$VERSION_FILE" | xargs)

if [ -z "$VERSION" ]; then
    echo "❌ 错误: $VERSION_FILE 文件内容为空！"
    exit 1
fi

REGISTRY="core.peopleurl.cn/token-hub/new-api"
IMAGE_TAG="${REGISTRY}:${VERSION}"

echo "=================================================="
echo "🐳 开始构建 Docker 镜像"
echo "📌 版本号: ${VERSION}"
echo "🏷️  镜像 Tag: ${IMAGE_TAG}"
echo "=================================================="

# 执行 Docker 构建（指定 amd64 架构以适配生产 K8s 环境）
docker build -t "${IMAGE_TAG}" --platform linux/amd64 .

echo "=================================================="
echo "✅ Docker 镜像构建完成: ${IMAGE_TAG}"
echo "=================================================="

# 如果传入了 --push 参数，自动推送到镜像仓库
if [ "$1" == "--push" ]; then
    echo "🚀 正在推送镜像到远程仓库..."
    docker push "${IMAGE_TAG}"
    echo "🎉 镜像推送成功: ${IMAGE_TAG}"
fi
