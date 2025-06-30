# Docker Hub 镜像使用说明

## 镜像信息

- **镜像名称**: `knktc/tiny-requestbin`
- **标签**: 
  - `latest`: 最新版本
  - `v{version}`: 特定版本（如 `v1.0.0`）

## 使用方法

### 1. 基本使用

```bash
# 运行默认配置（端口 3000）
docker run -p 3000:3000 knktc/tiny-requestbin

# 访问 http://localhost:3000
```

### 2. 自定义配置

```bash
# 自定义端口和参数
docker run -p 8080:8080 knktc/tiny-requestbin -port 8080 -listen 0.0.0.0 -max 1000

# 启用 CLI 模式
docker run -p 3000:3000 knktc/tiny-requestbin -cli
```

### 3. 使用 Docker Compose

```yaml
version: '3.8'
services:
  tiny-requestbin:
    image: knktc/tiny-requestbin:latest
    ports:
      - "3000:3000"
    command: ["-port", "3000", "-listen", "0.0.0.0", "-max", "1000"]
    restart: unless-stopped
```

运行：
```bash
docker-compose up -d
```

## 镜像特点

- **轻量级**: 基于 `scratch` 基础镜像，镜像大小约 10MB
- **多架构**: 支持 `linux/amd64` 和 `linux/arm64`
- **安全**: 静态编译，无外部依赖
- **快速**: 启动时间不到 1 秒

## 端口说明

- 默认端口: `3000`
- 可通过 `-port` 参数自定义
- 容器内部端口必须与 `-port` 参数一致

## 数据持久化

目前 Tiny RequestBin 将数据存储在内存中，容器重启后数据会丢失。这是设计上的选择，因为 RequestBin 主要用于临时调试和测试。

## 环境变量

目前镜像不支持环境变量配置，请使用命令行参数。

## 故障排除

### 1. 端口冲突
```bash
# 检查端口是否被占用
netstat -tulpn | grep :3000

# 使用其他端口
docker run -p 8080:8080 knktc/tiny-requestbin -port 8080
```

### 2. 权限问题
```bash
# 如果需要绑定特权端口（如 80），需要 root 权限
docker run --user root -p 80:80 knktc/tiny-requestbin -port 80 -listen 0.0.0.0
```

### 3. 容器无法访问
```bash
# 确保使用 0.0.0.0 而不是 127.0.0.1
docker run -p 3000:3000 knktc/tiny-requestbin -listen 0.0.0.0
```

## 更新镜像

```bash
# 拉取最新版本
docker pull knktc/tiny-requestbin:latest

# 停止并移除旧容器
docker stop tiny-requestbin
docker rm tiny-requestbin

# 启动新容器
docker run -d --name tiny-requestbin -p 3000:3000 knktc/tiny-requestbin
```
