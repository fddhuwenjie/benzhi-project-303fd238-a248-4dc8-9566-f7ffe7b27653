# BENZHI_README

## 项目说明
- 项目：benzhi-project-303fd238-a248-4dc8-9566-f7ffe7b27653
- 项目用途：潮声观测质检台是面向海洋研究团队的浮标声学观测质量审核与开放发布 Web 应用，提供从登记到发布冻结的可追溯流程。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 项目描述
- 项目名称：潮声观测质检台
- 项目介绍：面向海洋研究团队的浮标声学观测事件质量审核与开放发布 Web 应用。系统把一次观测从登记、校准证据收集、规则初筛、异常复核、独立签署推进到发布清单冻结，并保留可追溯审计时间线；按 standard 档位规划约 2140 行真实生产 Go 代码和 24 个生产 Go 文件。
- 项目概述：面向海洋研究团队的浮标声学观测事件质量审核与开放发布 Web 应用。系统把一次观测从登记、校准证据收集、规则初筛、异常复核、独立签署推进到发布清单冻结，并保留可追溯审计时间线；按 standard 档位规划约 2140 行真实生产 Go 代码和 24 个生产 Go 文件。
- 核心工作流：观测员创建声学观测事件并提交浮标与海域元数据，质检员补齐校准和原始片段摘要，系统执行规则初筛后由质检员标注异常并补交证据，独立审核人签署质量等级，最后生成并冻结可发布数据清单供研究者下载。
- 对外接口：浏览器中的单页工作台：Go 服务提供 HTML、CSS 和原生 JavaScript，用户在同一页面创建观测、上传证据摘要、处理审核任务、查看状态时间线并下载冻结发布包；服务支持 -addr=127.0.0.1:<port> 或 PORT 环境变量，默认监听 127.0.0.1:19081。

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...

cd /app && GOTOOLCHAIN=local go run ./cmd/server -addr=127.0.0.1:19081 -self-check

cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh

./build_benzhi_docker.sh benzhi-project-303fd238-a248-4dc8-9566-f7ffe7b27653-amd64 linux/amd64

./build_benzhi_docker.sh benzhi-project-303fd238-a248-4dc8-9566-f7ffe7b27653-arm64 linux/arm64

docker run -it benzhi-project-303fd238-a248-4dc8-9566-f7ffe7b27653-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server -addr=127.0.0.1:19081 -self-check`
