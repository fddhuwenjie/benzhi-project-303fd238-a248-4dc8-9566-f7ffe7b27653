# 潮声观测质检台

面向海洋研究团队的浮标声学观测事件质量审核与开放发布 Web 应用。系统支持观测登记、校准证据收集、规则初筛、异常复核、独立签署、发布清单冻结以及可追溯审计。

## 构建、运行与测试

```bash
go test ./...
go run ./cmd/server -addr=127.0.0.1:19081
go run ./cmd/server -addr=127.0.0.1:19081 -self-check
```

服务默认监听 `127.0.0.1:19081`，也可使用 `PORT` 环境变量或 `-addr` 参数调整。

HTTP API 位于 `/api/v1/cases`：个案登记与元数据更正、`evidence` 证据提交/替代及校准覆盖报告、`screen` 版本化规则初筛、`review/claim` 租约认领、带声明确认码的 `sign` 签署、`preview`/`freeze` 分片清单预览与冻结、按 `request_id` 过滤的 `audit` 分页审计，以及冻结后的 `download` 下载。规则配置可从 `/api/v1/rule-profiles` 查询，跨个案请求追踪入口为 `/api/v1/audit/trace?request_id=...`。
