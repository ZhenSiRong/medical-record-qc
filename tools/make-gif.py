"""
把录屏抽出的帧序列合成 README 内嵌的演示动图（GIF）。

为什么要统一调色板：GIF 每帧最多 256 色，若各帧独立量化会出现帧间色带抖动
（深色渐变背景上尤其明显）。做法是先用采样帧生成一张全局调色板，再让所有帧
套用同一张 —— 颜色稳定，体积也更小。

前置（本机环境坑）：Playwright 自带的 ffmpeg 是精简版，
只有 vp9 解码 + png 编码 + image2/webm muxer，**没有 gif 编码器**，
所以 GIF 只能走「抽帧 → PIL 统一量化」这条路。

用法（仓库根目录）：
    # 1) 抽帧（Playwright 自带 ffmpeg 即可）
    ffmpeg -i docs/screenshots/ccmcc-demo-20261106.webm -r 2 \
           -vf "scale=660:-1" .tmp-frames/frame_%03d.png
    # 2) 合成
    PYTHONPATH=<pillow 安装目录> python tools/make-gif.py .tmp-frames
"""
import glob
import os
import sys

from PIL import Image

COLORS = 128
SAMPLE_EVERY = 3      # 采样建调色板的步长
MAX_SAMPLE = 24       # 最多用多少帧建调色板（多了收益递减）
DURATION_MS = 500     # 2 fps → 每帧 500ms
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "docs", "screenshots", "demo.gif")


def main() -> int:
    frames_dir = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.getcwd(), ".tmp-frames")
    paths = sorted(glob.glob(os.path.join(frames_dir, "frame_*.png")))
    if not paths:
        print("NO_FRAMES in " + frames_dir)
        return 1

    # 1) 采样建全局调色板
    sample = paths[::SAMPLE_EVERY][:MAX_SAMPLE] or paths[:1]
    palette_src = Image.open(sample[0]).convert("RGB")
    for p in sample[1:]:
        palette_src = Image.blend(palette_src, Image.open(p).convert("RGB"), 0.5)
    palette = palette_src.quantize(colors=COLORS, method=Image.MEDIANCUT)

    # 2) 全帧套同一张调色板
    frames = []
    for p in paths:
        im = Image.open(p).convert("RGB")
        frames.append(im.quantize(palette=palette, dither=Image.Dither.NONE))

    out = os.path.abspath(OUT)
    frames[0].save(
        out,
        save_all=True,
        append_images=frames[1:],
        duration=DURATION_MS,
        loop=0,
        optimize=True,
        disposal=2,
    )
    print("GIF_OK frames=%d colors=%d size=%.0fKB -> %s" % (
        len(frames), COLORS, os.path.getsize(out) / 1024, out))
    return 0


if __name__ == "__main__":
    sys.exit(main())
