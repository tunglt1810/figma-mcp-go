#!/usr/bin/env node

const { spawnSync } = require("node:child_process");
const { existsSync } = require("node:fs");
const path = require("node:path");

const PLATFORM_MAP = { darwin: "darwin", linux: "linux", win32: "windows" };
const ARCH_MAP = { x64: "amd64", arm64: "arm64" };

const goos = PLATFORM_MAP[process.platform];
const goarch = ARCH_MAP[process.arch];

if (!goos || !goarch) {
  process.stderr.write(
    `[figma-mcp-go] Unsupported platform: ${process.platform}/${process.arch}\n`,
  );
  process.exit(1);
}

const binaryName =
  process.platform === "win32" ? "figma-mcp-go.exe" : "figma-mcp-go";
const binaryPath = path.join(__dirname, `${goos}-${goarch}`, binaryName);

if (!existsSync(binaryPath)) {
  process.stderr.write(
    "[figma-mcp-go] Binary not found. Try reinstalling: npm install @tunglt1810/figma-mcp-go\n",
  );
  process.exit(1);
}

const result = spawnSync(binaryPath, process.argv.slice(2), {
  stdio: "inherit",
});
process.exit(result.status ?? 1);
