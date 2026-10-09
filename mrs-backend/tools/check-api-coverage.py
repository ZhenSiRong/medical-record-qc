#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""API 契约覆盖度检查（可重复运行）

用途：防止 mrs-backend/docs/API.md 再次落后于实现（历史问题 D1）。
原理：从「真实路由清单」出发，检查每条已注册路由是否在 API.md 中被文档化。

真实路由清单的来源（二选一）：
  1) 默认读取 routes.actual.txt（由 `go test ./internal/router/ -run TestDumpRoutes` 生成）
  2) 若文件不存在，静默跳过并给出提示

用法：
    cd mrs-backend
    "C:\\Program Files\\Go\\bin\\go.exe" test ./internal/router/ -run TestDumpRoutes
    python tools/check-api-coverage.py            # 或 python3

退出码：0 = 全覆盖；1 = 存在未文档化路由；2 = 前置条件缺失
"""
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)                       # mrs-backend/
ACTUAL = os.path.join(ROOT, "routes.actual.txt")
API_MD = os.path.join(ROOT, "docs", "API.md")

# 通用 CRUD 的路径占位：API.md 里写作 /api/{资源}/page，需展开为 6 类资源
CRUD_RESOURCES = ["depts", "doctors", "dicts", "roles", "shelves", "users"]
CRUD_SUFFIXES = ["/page", "/:id", ""]
CRUD_METHODS = ["GET", "POST", "PUT", "DELETE"]


def load_actual():
    """读取真实路由清单 -> set of 'METHOD /path'"""
    if not os.path.exists(ACTUAL):
        return None
    out = set()
    with open(ACTUAL, encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            out.add(line)
    return out


def expand_crud_placeholders():
    """API.md 用 {资源} 概括的通用 CRUD，展开成具体端点集合。"""
    out = set()
    for res in CRUD_RESOURCES:
        out.add("GET /api/%s/page" % res)
        out.add("GET /api/%s/:id" % res)
        out.add("POST /api/%s" % res)
        out.add("PUT /api/%s/:id" % res)
        out.add("DELETE /api/%s/:id" % res)
    return out


def parse_documented():
    """从 API.md 提取被文档化的端点 -> set of 'METHOD /path'

    宽松解析：抓取全文所有 `/api/...` 字面量，再结合同一行出现的 HTTP 方法词。
    标题行形如 `### POST /api/xxx` 优先。
    """
    with open(API_MD, encoding="utf-8") as fh:
        text = fh.read()

    documented = set()
    methods = ("GET", "POST", "PUT", "DELETE", "PATCH")

    for raw in text.splitlines():
        line = raw.strip()
        # 去掉 markdown 标记，保留内容
        line = line.lstrip("#").lstrip(">").strip()
        # 只处理提到接口路径的行：
        #   ① 形如 `### POST /api/xxx` 或 `### GET /healthz` 的标题行
        #   ② 正文里以 `/api/` 开头的字面量
        if "/api/" not in line and not re.search(r"^(GET|POST|PUT|DELETE|PATCH)\s+/", line):
            continue
        # 该行声明的方法（可能多个，如 `GET`、`POST/PUT/DELETE`）
        line_methods = [m for m in methods if re.search(r"\b%s\b" % m, line)]
        paths = re.findall(r"/api/[A-Za-z0-9_\-/:{}]+", line)
        if not paths:
            # 非 /api 前缀的接口（如 /healthz）：按「方法 + 路径」整段提取
            for m in re.finditer(r"\b(GET|POST|PUT|DELETE|PATCH)\s+(/[A-Za-z0-9_\-/:{}]+)", line):
                paths.append(m.group(2))
                line_methods = [m.group(1)]
        for path in paths:
            path = path.rstrip(".,;、）)】")
            # 规范化：去掉查询串残留、去掉尾部斜杠
            path = path.split("?")[0].rstrip("/")
            if not path:
                continue
            # {资源} 占位单独展开
            if "{资源}" in path:
                for res in CRUD_RESOURCES:
                    expanded = path.replace("{资源}", res)
                    for m in (line_methods or CRUD_METHODS):
                        documented.add("%s %s" % (m, expanded))
                continue
            # :recordId / :recordNo 等参数名差异统一成 :id，便于与实现对齐
            norm = re.sub(r":(?!id\b)[A-Za-z_][A-Za-z0-9_]*", ":id", path)
            for m in (line_methods or ["GET"]):
                documented.add("%s %s" % (m, norm))
                if norm != path:
                    documented.add("%s %s" % (m, path))
    return documented


def norm(p):
    """把实现侧路径也做同样的参数名归一化。"""
    return re.sub(r":(?!id\b)[A-Za-z_][A-Za-z0-9_]*", ":id", p)


def main():
    actual = load_actual()
    if actual is None:
        print("[skip] 未找到 %s" % ACTUAL)
        print("       先生成：cd mrs-backend && go test ./internal/router/ -run TestDumpRoutes")
        return 2

    documented = parse_documented()
    documented |= expand_crud_placeholders()

    actual_norm = set()
    for item in actual:
        method, _, path = item.partition(" ")
        actual_norm.add("%s %s" % (method, norm(path)))

    missing = sorted(a for a in actual_norm if a not in documented)

    print("实际注册路由：%d" % len(actual_norm))
    print("API.md 覆盖： %d" % len(documented & actual_norm))
    if actual_norm:
        print("覆盖率：      %.1f%%" % (100.0 * len(documented & actual_norm) / len(actual_norm)))
    print("")
    if missing:
        print("未在 API.md 中记录的路由（%d 条）：" % len(missing))
        for m in missing:
            print("  - %s" % m)
        return 1

    print("OK：全部已注册路由均已在 API.md 中记录。")
    return 0


if __name__ == "__main__":
    sys.exit(main())
