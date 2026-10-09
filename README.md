# 病案管理系统（mrs）骨架

管理住院病案的**归档、编目、借阅、统计**，支持**病案首页数据的上报**。

- 设计文档：[DESIGN.md](DESIGN.md)（需求域、三条状态机、表结构说明、12 节课件对照）
- 后端：Go + Gin + GORM + PostgreSQL + Redis（`mrs-backend/`）
- 前端：由同事另行开发，集成依据 [mrs-backend/docs/API.md](mrs-backend/docs/API.md)
- 部署：Docker Compose

## 一键启动

```bash
docker compose up -d --build
# 表结构由 postgres 容器首次初始化（mrs-backend/sql/schema.sql）
# 后端空库时自动灌种子数据（管理员、ICD 编码库、质控规则、演示病案）
curl http://localhost:8080/healthz
```

账号：`admin / admin123`（病案管理员）、`deptuser / 123456`（科室用户）。

## 本地开发（不用 Docker，免管理员）

```bash
# 1. 数据库：两种方式任选
#    a) 本机已有 PostgreSQL：建库 mrs、用户 mrs/mrs123456 即可
#    b) 免安装便携版（tools/pg/，zonky 打包的 PG16 精简二进制）：
cd tools/pg
bin/initdb.exe -D ../../data/pgdata -U mrs --pwfile=pwfile.txt -E UTF8 -A scram-sha-256 --no-locale
bin/pg_ctl.exe -D ../../data/pgdata -l ../../data/pg.log -o "-p 5432" start > ../../data/pgctl.out 2>&1
cd ../.. && go run ./cmd/createdb      # 首次建库（表结构由后端 AutoMigrate 自动创建）

# 2. 启动后端（空库自动灌种子；Redis 缺失自动降级直查）
cd mrs-backend && go run .

# 3. 前端：两种方式
#    a) 联调模式（任意静态服务器，适配层自动指向 127.0.0.1:8080）：
cd .. && python -m http.server 8090 --bind 127.0.0.1
#       打开 http://127.0.0.1:8090/frontend/index.html
#    b) 生产形态：后端 go:embed 前端单二进制（P2 收口项）
```

账号：`admin / admin123`（病案管理员）、`deptuser / 123456`（科室医师）。
种子数据约 87 份病案（近 6 个月、状态加权）、12 科室、12 医师、35 条 ICD 码表、19 张借阅单、2 个历史上报批次——重灌：`go run ./cmd/createdb drop && go run ./cmd/createdb` 后重启后端。

### WSL2 / Docker

`tools/enable-wsl2.cmd` 双击运行（UAC 点「是」）启用虚拟机平台，重启后：
`wsl --set-default-version 2 && wsl --install -d Ubuntu`，随后可在 WSL 内使用 docker compose。

## 验证

```bash
cd mrs-backend
go build ./...   # 编译
go vet ./...     # 静态检查
go test ./...    # 路由装配冒烟测试
```

## 目录速览

```
mrs-backend/
├── main.go  config.yaml        # 入口与配置（MRS_* 环境变量可覆盖）
├── sql/schema.sql              # PostgreSQL 表结构（唯一事实来源）
├── docs/API.md                 # 前端集成契约（接口清单 + 示例）
├── pkg/resp/                   # 统一响应 {code,msg,data}、分页
└── internal/
    ├── config/ cache/ seed/    # 配置 / Redis 缓存 / 空库种子
    ├── model/                  # GORM 实体 + 三条状态机常量
    ├── middleware/             # CORS / JWT / 角色校验 / 操作日志审计
    ├── handler/                # 各业务模块接口（对应 12 节课件）
    ├── service/                # 质控引擎 / 上报防腐层 / 逾期扫描
    └── router/                 # 路由装配
```

## 骨架中已留的占位（调研确认后填充）

| 事项 | 位置 |
|---|---|
| 借阅审批层级数、加急规则 | `config.yaml borrow.approve_levels` |
| 官方上报接口（现为本地 XML 桩） | `internal/service/reportclient.go` |
| 质控规则完整清单 | `qc_rule` 表种子数据 + `qcengine.go checkLogic` |
| 统计口径与报表导出 | `internal/handler/statistics.go Export`（501） |
| 科室级数据隔离 | `sys_role.data_scope` + 查询层（未生效） |
