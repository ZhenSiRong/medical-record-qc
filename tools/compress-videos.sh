#!/usr/bin/env bash
# 批量压缩演示录屏（VP9 / CRF 压到 CDN 可接受的体积）
#
# 为什么需要：README 里的视频走 jsDelivr CDN，原片 5–11 MB 会让首屏加载很慢。
# 但 ffmpeg 是 Playwright 的精简版（无 libx264、无 concat），**只有 libvpx-vp9 编码器**，
# 所以只能用 VP9 + CRF 控质量。CRF 越高体积越小、画质越糊，42 是「文字仍清晰」的档位。
#
# 用法：bash tools/compress-videos.sh [目标体积上限MB]
set -u
ROOT="D:/ZcodeWorktree/yrzk"
FF="C:/Users/12743/AppData/Local/ms-playwright/ffmpeg-1013/ffmpeg-win64.exe"
MAX_MB="${1:-3}"
cd "$ROOT" || exit 1

compress_one() {
  local src="$1"
  [ -f "$src" ] || return 0
  local dst="${src%.webm}.min.webm"
  local sz
  sz=$(du -k "$src" | cut -f1)
  # 已经是 min 版就跳过；小于上限就跳过
  case "$src" in *.min.webm) return 0 ;; esac
  if [ "$sz" -lt $((MAX_MB * 1024)) ]; then
    echo "SKIP(small) $(basename "$src") $((sz/1024))MB"
    return 0
  fi
  echo "COMPRESS $(basename "$src") $((sz/1024))MB -> $dst"
  "$FF" -hide_banner -loglevel error -y -i "$src" \
    -c:v libvpx-vp9 -crf 42 -b:v 0 -row-mt 1 -pix_fmt yuv420p -deadline good -cpu-used 2 \
    "$dst" 2>&1 | head -3
  if [ -f "$dst" ]; then
    local nsz
    nsz=$(du -k "$dst" | cut -f1)
    echo "  OK $((nsz/1024))MB (was $((sz/1024))MB)"
    # 压完用小的替掉大的，并删掉中间文件
    mv "$dst" "$src"
  else
    echo "  FAILED keep original"
    rm -f "$dst"
  fi
}

for f in \
  docs/screenshots/tour/demo.webm \
  docs/screenshots/ccmcc-demo-20261106.webm \
  docs/screenshots/archive/demo.webm \
  docs/screenshots/borrow/demo.webm \
  docs/screenshots/stats/demo.webm \
  docs/screenshots/quality/demo.webm \
  docs/screenshots/report/demo.webm \
  docs/screenshots/rectify/demo.webm \
  docs/screenshots/rectify-clinical/demo-clinical.webm \
  docs/screenshots/ai-assist/demo.webm
do
  compress_one "$f"
done

# 清掉 agent 留下的压缩中间产物
rm -f docs/screenshots/*/.compressed-*.webm docs/screenshots/*/.crf*-*.webm
echo "DONE"
ls -lh docs/screenshots/*/*.webm 2>/dev/null | awk '{print $5, $9}'
