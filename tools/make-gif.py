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
DEFAULT_OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "docs", "screenshots", "demo.gif")
# 开头白帧判定阈值：平均亮度高于此值视为「空白起幅帧」
BRIGHT_SKIP = 240


def _is_blank(path) -> bool:
    """判断该帧是否为纯白起幅帧（Playwright 录屏开头会有几帧空白）。

    这不是洁癖：README 首屏就是动图第一帧，开场若是一张白图，
    观众点进来看到的第一眼就是「什么都没有」。
    """
    try:
        im = Image.open(path).convert("L")
        w, h = im.size
        # 只采样像素（缩小到 1/16 面积即可判定亮度，够快）
        im = im.resize((max(1, w // 16), max(1, h // 16)))
        total = 0
        count = 0
        for v in im.get_flattened_data() if hasattr(im, "get_flattened_data") else im.getdata():
            total += v
            count += 1
        return count > 0 and (total / count) > BRIGHT_SKIP
    except Exception as e:  # 判定失败就不丢帧，宁可留白也不误删
        print("BLANK_CHECK_FAILED %s %s" % (path, e))
        return False


def main() -> int:
    frames_dir = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.getcwd(), ".tmp-frames")
    # 第二个参数可指定输出路径（相对仓库根或绝对路径）—— 多段演示各自出图时用得上
    out_arg = sys.argv[2] if len(sys.argv) > 2 else DEFAULT_OUT
    paths = sorted(glob.glob(os.path.join(frames_dir, "frame_*.png")))
    if not paths:
        print("NO_FRAMES in " + frames_dir)
        return 1

    # 0) 丢掉开头的空白起幅帧（见 _is_blank 注释）
    dropped = 0
    while paths and _is_blank(paths[0]):
        paths.pop(0)
        dropped += 1
    if dropped:
        print("SKIP_BLANK_HEAD %d frames" % dropped)
    if not paths:
        print("ALL_FRAMES_BLANK")
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

    out = os.path.abspath(out_arg)
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
