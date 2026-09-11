#!/usr/bin/env node
// Launcher for the promptcc release binary. The npm package holds no code
// of its own: on first run it downloads the binary matching this package's
// version from GitHub releases, caches it next to this file, and execs it.
"use strict";

const fs = require("fs");
const os = require("os");
const path = require("path");
const https = require("https");
const { spawnSync } = require("child_process");

const pkg = require("../package.json");
const REPO = "halleck45/promptcc";

function target() {
  const osName = { linux: "linux", darwin: "darwin", win32: "windows" }[process.platform];
  const arch = { x64: "amd64", arm64: "arm64" }[process.arch];
  if (!osName || !arch) {
    console.error(`promptcc: no release binary for ${process.platform}/${process.arch}.`);
    console.error("Build from source instead: go install github.com/halleck45/promptcc/cmd/promptcc@latest");
    process.exit(2);
  }
  if (osName === "windows" && arch === "arm64") {
    console.error("promptcc: Windows on ARM has no release binary yet. Use WSL or build from source.");
    process.exit(2);
  }
  const asset = `promptcc_${osName}_${arch}${osName === "windows" ? ".exe" : ""}`;
  return { asset, exe: osName === "windows" ? "promptcc.exe" : "promptcc" };
}

function download(url, dest, redirects = 0) {
  return new Promise((resolve, reject) => {
    if (redirects > 5) return reject(new Error("too many redirects"));
    https
      .get(url, { headers: { "User-Agent": `promptcc-npm/${pkg.version}` } }, (res) => {
        if ([301, 302, 303, 307, 308].includes(res.statusCode) && res.headers.location) {
          res.resume();
          return resolve(download(res.headers.location, dest, redirects + 1));
        }
        if (res.statusCode !== 200) {
          res.resume();
          return reject(new Error(`HTTP ${res.statusCode} for ${url}`));
        }
        const tmp = dest + ".part";
        const out = fs.createWriteStream(tmp, { mode: 0o755 });
        res.pipe(out);
        out.on("finish", () => out.close(() => { fs.renameSync(tmp, dest); resolve(); }));
        out.on("error", reject);
      })
      .on("error", reject);
  });
}

async function ensureBinary() {
  const { asset, exe } = target();
  const dir = path.join(__dirname, "..", "vendor", pkg.version);
  const bin = path.join(dir, exe);
  if (fs.existsSync(bin)) return bin;
  fs.mkdirSync(dir, { recursive: true });
  const url = `https://github.com/${REPO}/releases/download/v${pkg.version}/${asset}`;
  process.stderr.write(`promptcc: downloading ${asset} (v${pkg.version})...\n`);
  try {
    await download(url, bin);
  } catch (err) {
    console.error(`promptcc: download failed: ${err.message}`);
    console.error(`Grab it manually from https://github.com/${REPO}/releases`);
    process.exit(2);
  }
  return bin;
}

ensureBinary().then((bin) => {
  const result = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });
  if (result.error) {
    console.error(`promptcc: ${result.error.message}`);
    process.exit(2);
  }
  process.exit(result.status === null ? 1 : result.status);
});
