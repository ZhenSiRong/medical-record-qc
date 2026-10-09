"""render-spec.py —— 把 SPEC.md 渲染为带样式的 HTML 预览（答辩/评审时浏览器直接看）

用法（项目根目录）：python tools/render-spec.py
产物：spec-preview.html（与 SPEC.md 同目录，图片走相对路径）

依赖：markdown 库（pip install markdown）
"""
import os
import sys

import markdown

STYLE = """
body{font-family:"Microsoft YaHei",system-ui,sans-serif;max-width:1080px;margin:0 auto;
     padding:32px 24px;line-height:1.75;color:#1f2733;background:#fff}
h1{font-size:28px;border-bottom:3px solid #1898f8;padding-bottom:12px}
h2{font-size:21px;margin-top:38px;padding-left:10px;border-left:5px solid #009088}
h3{font-size:17px;margin-top:26px;color:#0f5a52}
h4{font-size:15.5px;margin-top:22px;color:#33415c}
table{border-collapse:collapse;width:100%;margin:14px 0;font-size:14px}
th,td{border:1px solid #dfe6f0;padding:8px 11px;text-align:left;vertical-align:top}
th{background:#f2f7ff}
code{background:#eef2fa;padding:1px 6px;border-radius:4px;font-family:Consolas,monospace;
     font-size:13px;color:#0b5bd3}
pre{background:#f6f8fc;border:1px solid #e3e9f4;border-radius:8px;padding:14px;overflow-x:auto}
pre code{background:none;color:#243447}
img{max-width:100%;height:auto;border:1px solid #e3e9f4;border-radius:8px;
    margin:10px 0 4px;box-shadow:0 2px 10px rgba(20,35,60,.08)}
em{color:#5a6779;font-size:13.5px}
blockquote{border-left:4px solid #1898f8;background:#f4f9ff;margin:14px 0;
           padding:10px 16px;color:#33415c}
"""

TEMPLATE = """<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8">
<title>{title}</title><style>{style}</style></head><body>{body}</body></html>"""


def main() -> int:
    root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    src_path = os.path.join(root, "SPEC.md")
    if not os.path.isfile(src_path):
        print(f"找不到 {src_path}", file=sys.stderr)
        return 1

    with open(src_path, encoding="utf-8") as f:
        src = f.read()

    body = markdown.markdown(src, extensions=["tables", "fenced_code"])
    html = TEMPLATE.format(title="SPEC · 病案管理系统", style=STYLE, body=body)

    out_path = os.path.join(root, "spec-preview.html")
    with open(out_path, "w", encoding="utf-8") as f:
        f.write(html)
    print(f"已生成 {out_path}（{len(html)} 字符）")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
