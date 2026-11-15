#!/usr/bin/env node
// The `webnak` npm package: runs the Webnak CLI binary for this platform.
//
// On first use it downloads the matching binary from the GitHub release of
// this exact version, checks it against the release's published SHA-256
// checksums, and keeps it in a cache. No install scripts run.
"use strict";

const fs = require("fs");
const os = require("os");
const path = require("path");
const https = require("https");
const crypto = require("crypto");
const { spawn } = require("child_process");

const VERSION = require("../package.json").version;
const REPO = "sathninduk/webnak-cli";

function target() {
  const plat = { darwin: "darwin", linux: "linux", win32: "windows" }[process.platform];
  const arch = { x64: "amd64", arm64: "arm64" }[process.arch];
  if (!plat || !arch) {
    throw new Error(`webnak has no build for ${process.platform}/${process.arch}. See https://github.com/${REPO}`);
  }
  return `webnak_${plat}_${arch}${plat === "windows" ? ".exe" : ""}`;
}

function cacheDir() {
  if (process.env.WEBNAK_CACHE_DIR) return process.env.WEBNAK_CACHE_DIR;
  if (process.platform === "win32") return path.join(process.env.LOCALAPPDATA || path.join(os.homedir(), "AppData", "Local"), "webnak");
  return path.join(process.env.XDG_CACHE_HOME || path.join(os.homedir(), ".cache"), "webnak");
}

function get(url, redirects = 5) {
  return new Promise((resolve, reject) => {
    https.get(url, { headers: { "User-Agent": `webnak-npm/${VERSION}` } }, (res) => {
      if ([301, 302, 303, 307, 308].includes(res.statusCode) && res.headers.location && redirects > 0) {
        res.resume();
        return resolve(get(new URL(res.headers.location, url).toString(), redirects - 1));
      }
      if (res.statusCode !== 200) {
        res.resume();
        return reject(new Error(`download failed (${res.statusCode}) for ${url}`));
      }
      const chunks = [];
      res.on("data", (c) => chunks.push(c));
      res.on("end", () => resolve(Buffer.concat(chunks)));
      res.on("error", reject);
    }).on("error", reject);
  });
}

async function ensureBinary() {
  if (process.env.WEBNAK_BINARY) return process.env.WEBNAK_BINARY;
  const name = target();
  const dir = path.join(cacheDir(), VERSION);
  const bin = path.join(dir, name);
  if (fs.existsSync(bin)) return bin;

  const base = `https://github.com/${REPO}/releases/download/v${VERSION}`;
  process.stderr.write(`Downloading webnak ${VERSION} for ${name.replace(/^webnak_|\.exe$/g, "")}…\n`);
  const [sums, body] = await Promise.all([get(`${base}/checksums.txt`), get(`${base}/${name}`)]);
  const want = sums.toString("utf8").split("\n").map((l) => l.trim().split(/\s+/)).find((p) => p[1] === name);
  if (!want) throw new Error(`no checksum published for ${name}`);
  const got = crypto.createHash("sha256").update(body).digest("hex");
  if (got !== want[0]) throw new Error(`checksum mismatch for ${name}: refusing to run it`);

  fs.mkdirSync(dir, { recursive: true });
  const tmp = `${bin}.${process.pid}.tmp`;
  fs.writeFileSync(tmp, body, { mode: 0o755 });
  fs.renameSync(tmp, bin);
  return bin;
}

ensureBinary()
  .then((bin) => {
    const child = spawn(bin, process.argv.slice(2), { stdio: "inherit", windowsHide: true });
    for (const sig of ["SIGINT", "SIGTERM", "SIGHUP"]) {
      process.on(sig, () => { try { child.kill(sig); } catch (_) { /* gone */ } });
    }
    child.on("exit", (code, signal) => {
      if (signal) process.kill(process.pid, signal);
      else process.exit(code === null ? 1 : code);
    });
    child.on("error", (err) => { console.error(`webnak: ${err.message}`); process.exit(1); });
  })
  .catch((err) => {
    console.error(`webnak: ${err.message}`);
    process.exit(1);
  });
