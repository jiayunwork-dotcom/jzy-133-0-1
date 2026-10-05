# SPC 均值–极差图工具

车间实时统计过程控制：录入测量值 → 均值–极差控制图当场更新 → Nelson 判异规则立即标红。

- 后端 Go 1.22 + Echo + PostgreSQL 16
- 前端 Vue 3 + Vite（纯 SVG 控制图）
- docker compose 三容器：`backend` / `frontend(nginx:1.27-alpine)` / `postgres:16-alpine`

## 启动

```bash
docker compose up --build
# 浏览器打开 http://localhost:8088
```

## 使用

1. 新建监控对象：填名称、子组容量（2–10）、规格上下限（可单侧）、勾选判异规则。
2. 录入：录入区可逐个录单值，或粘贴一整列数（按容量自动分组）。
3. 至少 20 个子组后，点「冻结基准」；中心线与控制限当场冻结，之后新点只按冻结限判定。
4. 告警点在均值图上按规则着色，右侧列出规则编号与涉及子组。
5. 显式「重新基准」会把旧限与生效区间留档，历史点仍按当时的限显示。

详细设计（并发切组、冻结/重基准、判异一致性、校验、测试）见
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)。

## 后端测试

```bash
cd backend && go test ./...
```
