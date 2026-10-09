"""serve.py —— 前端联调静态服务器（带 no-cache 头，避免浏览器启发式缓存旧 JS）
用法：python serve.py [端口]   （默认 8090，在仓库根目录运行）
"""
import mimetypes
import os
import sys
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer

mimetypes.add_type("font/woff2", ".woff2")
mimetypes.add_type("font/woff", ".woff")
mimetypes.add_type("text/css", ".css")


class NoCacheHandler(SimpleHTTPRequestHandler):
    def end_headers(self):
        self.send_header("Cache-Control", "no-cache, no-store, must-revalidate")
        self.send_header("Pragma", "no-cache")
        self.send_header("Expires", "0")
        super().end_headers()

    def log_message(self, fmt, *args):
        sys.stdout.write("%s - %s\n" % (self.address_string(), fmt % args))
        sys.stdout.flush()


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8090
    os.chdir(os.path.dirname(os.path.abspath(__file__)))
    print(f"静态服务已启动 http://127.0.0.1:{port}/ （Cache-Control: no-cache）")
    ThreadingHTTPServer(("127.0.0.1", port), NoCacheHandler).serve_forever()
