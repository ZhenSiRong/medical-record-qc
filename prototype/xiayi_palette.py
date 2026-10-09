# -*- coding: utf-8 -*-
"""xiayi_palette.py — 从侠医工作站截图拾色 + 量布局比例（仪器测量，非目测）"""
import sys
from collections import Counter
from PIL import Image

sys.stdout.reconfigure(encoding="utf-8")
img = Image.open("prototype/reference/xiayi-workstation.jpg").convert("RGB")
W, H = img.size
px = img.load()
print(f"图像尺寸: {W}x{H}")

def hexc(c): return "#%02x%02x%02x" % c

def dominant(box, n=6, sat_min=0):
    """区域内主色聚类：按出现次数排序（可选最低饱和度过滤）"""
    x0, y0, x1, y1 = box
    cnt = Counter()
    for y in range(y0, y1, 2):
        for x in range(x0, x1, 2):
            r, g, b = px[x, y]
            mx, mn = max(r, g, b), min(r, g, b)
            sat = 0 if mx == 0 else (mx - mn) / mx
            if sat >= sat_min:
                cnt[(r // 8 * 8, g // 8 * 8, b // 8 * 8)] += 1
    return [(hexc(c), k) for c, k in cnt.most_common(n)]

def avg(box):
    x0, y0, x1, y1 = box
    rs = gs = bs = t = 0
    for y in range(y0, y1):
        for x in range(x0, x1):
            r, g, b = px[x, y]
            rs += r; gs += g; bs += b; t += 1
    return hexc((rs // t, gs // t, bs // t))

print("\n== 1. 布局边界（列剖面：从左到右找暗色侧栏结束点）==")
dark_end = 0
for x in range(0, 600):
    col = [px[x, y] for y in range(200, 900, 10)]
    lum = sum(0.299 * r + 0.587 * g + 0.114 * b for r, g, b in col) / len(col)
    if lum < 90:
        dark_end = x
print(f"深色主导航右边界 x={dark_end}px，占比 {dark_end/W*100:.1f}%")

# 左树面板右边界：在 x=dark_end..700 找树区滚动条/留白（取中部行扫描颜色跳变）
# 右侧信息面板左边界：从右往左找浅灰面板起点
rp_start = W
for x in range(W - 1, W - 500, -1):
    col = [px[x, y] for y in range(180, 300, 4)]
    m = tuple(sum(c[i] for c in col) // len(col) for i in range(3))
    if m != (255, 255, 255):
        rp_start = x
print(f"右侧信息面板起点 x≈{rp_start}px，占比 {(W-rp_start)/W*100:.1f}%")

print("\n== 2. 深色主导航（0..边界）主色 ==")
for c, k in dominant((0, 60, dark_end, 1000), n=5):
    print(f"  {c}  x{k}")

print("\n== 3. 激活菜单项「加工病案」背景 (y≈70-100) ==")
print("  均色:", avg((4, 70, dark_end - 4, 100)))
for c, k in dominant((4, 70, dark_end - 4, 100), n=4, sat_min=0.25):
    print(f"  高饱和: {c}  x{k}")

print("\n== 4. 顶部工具栏 (y≈122-156) 主色/按钮态 ==")
tb = (dark_end + 20, 122, 1760, 156)
print("  工具栏底色:", avg((210, 125, 260, 152)))
for c, k in dominant(tb, n=8, sat_min=0.35):
    print(f"  高饱和: {c}  x{k}")
print("  「拍摄完成」橙钮:", avg((815, 126, 890, 152)), "| 主色:", dominant((815, 126, 890, 152), 3, 0.4)[:2])
print("  「扫描跟踪」绿钮:", avg((1596, 126, 1668, 152)), "| 主色:", dominant((1596, 126, 1668, 152), 3, 0.4)[:2])

print("\n== 5. 左树选中项「入院记录-0001」(y≈322-342) ==")
print("  均色:", avg((300, 322, 420, 342)))
for c, k in dominant((300, 322, 420, 342), n=4, sat_min=0.2):
    print(f"  高饱和: {c}  x{k}")

print("\n== 6. 扫描定位绿框（实时图像边框 y≈235-240, x≈1130-1590）==")
for c, k in dominant((1128, 233, 1590, 243), n=4, sat_min=0.5):
    print(f"  {c}  x{k}")

print("\n== 7. 右侧面板分类按钮区 (y≈550-580, x≈1640-1910) ==")
for c, k in dominant((1640, 550, 1910, 580), n=4, sat_min=0.15):
    print(f"  {c}  x{k}")

print("\n== 8. 工具栏几何 ==")
# 工具栏高度：x=1000 列，y 100..180 找非白行
ys = [y for y in range(100, 190) if px[1000, y] != (255, 255, 255) and sum(px[1000, y]) < 740]
print(f"  工具栏内容行范围 y: {min(ys)}..{max(ys)}，高 {max(ys)-min(ys)}px，占高 {((max(ys)-min(ys))/H)*100:.1f}%")

print("\n== 9. 顶部标签栏几何 ==")
print(f"  深色栏宽:dark_end={dark_end}px({dark_end/W*100:.1f}%) | 视口基准 1280px 时 ≈ {dark_end/W*1280:.0f}px")
