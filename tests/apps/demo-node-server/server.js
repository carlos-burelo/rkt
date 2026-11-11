// Node stdlib, sin dependencias: sirve VERSION en 127.0.0.1:$PORT.
const http = require("http");
const fs = require("fs");
const path = require("path");

let version = "sin-version";
try {
  const v = fs.readFileSync(path.join(__dirname, "VERSION"), "utf8").trim();
  if (v) version = v;
} catch {
  // sin VERSION: placeholder
}

const port = parseInt(process.env.PORT || "8080", 10);
http
  .createServer((req, res) => {
    res.writeHead(200, { "Content-Length": Buffer.byteLength(version) });
    res.end(version);
  })
  .listen(port, "127.0.0.1");
