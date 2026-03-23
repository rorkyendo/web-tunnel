const express = require("express");
const http = require("http");
const WebSocket = require("ws");
const { v4: uuidv4 } = require("uuid");

const app = express();
const server = http.createServer(app);
const wss = new WebSocket.Server({ server });

const ROOT_DOMAIN = process.env.TUNNEL_DOMAIN || "medandigital.dev";
const AUTH_TOKEN = process.env.MTUNNEL_AUTH_TOKEN || "";
const REQUEST_TIMEOUT_MS = Number(process.env.TUNNEL_REQUEST_TIMEOUT_MS || 25000);

const tunnels = {};
const pending = {};

function cleanupPendingBySubdomain(subdomain) {
  for (const [id, item] of Object.entries(pending)) {
    if (item.subdomain !== subdomain) continue;
    clearTimeout(item.timeout);
    item.res.status(502).send("Tunnel disconnected");
    delete pending[id];
  }
}

function isValidSubdomain(input) {
  return /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(input);
}

// WS CONNECTION
wss.on("connection", (ws) => {

  ws.isAlive = true;
  ws.on("pong", () => {
    ws.isAlive = true;
  });

  ws.on("message", (msg) => {
    let data;
    try {
      data = JSON.parse(msg);
    } catch {
      ws.send(JSON.stringify({ type: "error", message: "Invalid JSON" }));
      return;
    }

    // REGISTER
    if (data.type === "register") {
      let sub = data.subdomain || uuidv4().slice(0, 6);
      sub = String(sub).toLowerCase();

      if (!isValidSubdomain(sub)) {
        ws.send(JSON.stringify({ type: "error", message: "Invalid subdomain" }));
        return;
      }

      if (AUTH_TOKEN && data.token !== AUTH_TOKEN) {
        ws.send(JSON.stringify({ type: "error", message: "Unauthorized token" }));
        ws.close(1008, "Unauthorized");
        return;
      }

      if (tunnels[sub]) {
        ws.send(JSON.stringify({ type: "error", message: "Subdomain used" }));
        return;
      }

      ws.subdomain = sub;
      tunnels[sub] = ws;

      console.log("✅ Registered:", sub);

      ws.send(JSON.stringify({
        type: "assigned",
        url: `https://${sub}.${ROOT_DOMAIN}`
      }));
    }

    if (data.type === "pong") {
      ws.isAlive = true;
    }

    // RESPONSE
    if (data.type === "response") {
      const item = pending[data.id];
      if (!item) return;

      clearTimeout(item.timeout);
      delete pending[data.id];

      const headers = data.headers || {};
      for (const key of Object.keys(headers)) {
        if (key.toLowerCase() === "content-length") {
          delete headers[key];
        }
      }

      item.res.writeHead(data.status || 200, headers);
      item.res.end(Buffer.from(data.body || "", "base64"));
    }
  });

  ws.on("close", () => {
    if (ws.subdomain) {
      cleanupPendingBySubdomain(ws.subdomain);
      delete tunnels[ws.subdomain];
      console.log("❌ Disconnected:", ws.subdomain);
    }
  });

  ws.on("error", (err) => {
    console.error("WS error:", err.message);
  });
});

const heartbeat = setInterval(() => {
  for (const ws of wss.clients) {
    if (ws.isAlive === false) {
      ws.terminate();
      continue;
    }

    ws.isAlive = false;
    try {
      ws.ping();
      ws.send(JSON.stringify({ type: "ping" }));
    } catch {
      ws.terminate();
    }
  }
}, 30000);

wss.on("close", () => {
  clearInterval(heartbeat);
});

// HTTP HANDLER
app.use((req, res) => {
  const host = req.headers.host || "";
  const sub = host.split(".")[0];
  const client = tunnels[sub];

  if (!client) return res.status(404).send("Tunnel not found");

  if (client.readyState !== WebSocket.OPEN) {
    return res.status(502).send("Tunnel not ready");
  }

  const id = uuidv4();
  const timeout = setTimeout(() => {
    if (!pending[id]) return;
    pending[id].res.status(504).send("Tunnel timeout");
    delete pending[id];
  }, REQUEST_TIMEOUT_MS);

  pending[id] = { res, subdomain: sub, timeout };

  let body = [];

  req.on("data", chunk => body.push(chunk));

  req.on("end", () => {
    const headers = {
      ...req.headers,
      host: host,
      "x-forwarded-host": host,
      "x-forwarded-proto": "https",
      "x-forwarded-port": "443",
      "x-forwarded-for": req.socket.remoteAddress || ""
    };

    try {
      client.send(JSON.stringify({
        type: "request",
        id,
        method: req.method,
        path: req.url,
        headers,
        body: Buffer.concat(body).toString("base64")
      }));
    } catch {
      clearTimeout(timeout);
      delete pending[id];
      res.status(502).send("Failed to forward request");
    }
  });
});

server.listen(3000, () => {
  console.log("🚀 Tunnel server running");
});