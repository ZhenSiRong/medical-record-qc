#!/usr/bin/env bash
# 录制脚本的一次性运行包装：把「起服务 → 跑脚本 → 收服务」放在同一个进程生命周期里。
#
# 为什么需要它：本机沙箱会在**每次工具调用结束时回收该调用派生的所有子进程**。
# 于是用 run_in_background 起的后端/前端，一到下一次工具调用就死了 ——
# 表现为「脚本跑到一半 fetch failed」或「curl 502」。
# 把三步塞进同一个 shell 调用里，服务就活到脚本跑完。
#
# 用法：bash tools/with-services.sh <录制脚本.js> [更多参数...]
set -u
ROOT="D:/ZcodeWorktree/yrzk"
cd "$ROOT" || exit 1

NODE="C:/Users/12743/.workbuddy/binaries/node/versions/22.22.2-6/node.exe"
export NODE_PATH="C:/Users/12743/.workbuddy/binaries/node/workspace/node_modules"

PG_PID=""
BE_PID=""
FE_PID=""

cleanup() {
  [ -n "$BE_PID" ] && kill "$BE_PID" 2>/dev/null
  [ -n "$FE_PID" ] && kill "$FE_PID" 2>/dev/null
  [ -n "$PG_PID" ] && kill "$PG_PID" 2>/dev/null
}
trap cleanup EXIT

echo "[with-services] 启动 PG..."
./tools/pg/bin/postgres.exe -D data/pgdata >/dev/null 2>&1 &
PG_PID=$!

# 等 PG 真正 ready（日志出现 ready to accept 才算）
for i in $(seq 1 30); do
  sleep 1
  if tail -5 data/pg.log 2>/dev/null | grep -q "ready to accept connections"; then
    # 若日志是旧的，确认端口真的在监听
    if (echo > /dev/tcp/127.0.0.1/5432) 2>/dev/null; then
      echo "[with-services] PG ready (${i}s)"; break
    fi
  fi
done
if ! (echo > /dev/tcp/127.0.0.1/5432) 2>/dev/null; then
  echo "[with-services] PG 启动失败"; exit 1
fi

echo "[with-services] 启动后端..."
( cd mrs-backend && ../data/mrs.exe -config config.yaml > ../data/backend.out.log 2>&1 ) &
BE_PID=$!

echo "[with-services] 启动前端..."
python -m http.server 8090 --bind 127.0.0.1 >/dev/null 2>&1 &
FE_PID=$!

# 等后端 ready（用 curl 直连，不走代理）
for i in $(seq 1 40); do
  sleep 1
  code=$(curl -s --noproxy '*' -o /dev/null -w "%{http_code}" --max-time 3 \
    -X POST http://127.0.0.1:8080/api/auth/login \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"admin123"}' 2>/dev/null)
  if [ "$code" = "200" ]; then
    echo "[with-services] 后端 ready (${i}s)"; break
  fi
done
if [ "$code" != "200" ]; then
  echo "[with-services] 后端启动失败 (code=$code)"; tail -20 data/backend.out.log 2>/dev/null; exit 1
fi

echo "[with-services] 运行: $@"
"$NODE" "$@"
RC=$?
echo "[with-services] 脚本退出码=$RC"
exit $RC
