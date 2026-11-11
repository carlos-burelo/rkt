import http.server
import os

BASE = os.path.dirname(os.path.abspath(__file__))


def version():
    try:
        with open(os.path.join(BASE, "VERSION"), encoding="utf-8") as f:
            v = f.read().strip()
            return v if v else "vacia"
    except OSError:
        return "sin-version"


VERSION = version()


class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        body = VERSION.encode()
        self.send_response(200)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *a):
        pass


port = int(os.environ.get("PORT", "8080"))
http.server.HTTPServer(("127.0.0.1", port), H).serve_forever()
