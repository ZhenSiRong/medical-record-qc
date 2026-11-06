# 行为规范：接口权限闸门
#
# 权威源：
#   - 接口契约：mrs-backend/docs/API.md §8 → PUT /api/front-pages/:recordId 标注（ADMIN）
#   - 路由装配：mrs-backend/internal/router/router.go（admin 组挂 RequireRole("ADMIN")）
#
# 断言层级：HTTP（真实全量路由 + 真实中间件链）。
#
# 与 front_page_other_diagnosis.feature 共用「无数据库桩」约定：
# 越过权限闸门且数据合法的请求会触达 nil DB（兑现为 500），
# 而 401 / 403 必须在进入业务逻辑之前就被拒绝。

Feature: 接口权限闸门

  作为系统所有者
  我想要写操作只能由有权限的角色执行
  以便临床侧不会绕过病案室直接改动首页数据

  Scenario: 未携带令牌的请求被拒绝
    When 未携带令牌保存病案首页
    Then 响应状态码应为 401

  Scenario: 科室医师无权保存病案首页
    Given 科室医师已登录
    When 保存病案首页
    Then 响应状态码应为 403

  Scenario: 病案室管理员可通过权限闸门
    Given 病案室管理员已登录
    When 保存病案首页
    Then 请求应越过校验并触达数据库层
