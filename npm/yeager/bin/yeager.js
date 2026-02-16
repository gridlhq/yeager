#!/usr/bin/env node

"use strict";

const { execFileSync } = require("child_process");
const path = require("path");
const os = require("os");

// Platform package mapping (esbuild pattern).
const PLATFORM_PACKAGES = {
  "darwin-arm64": "@yeager.sh/darwin-arm64",
  "darwin-x64": "@yeager.sh/darwin-x64",
  "linux-arm64": "@yeager.sh/linux-arm64",
  "linux-x64": "@yeager.sh/linux-x64",
  "win32-arm64": "@yeager.sh/win32-arm64",
  "win32-x64": "@yeager.sh/win32-x64",
};

function getBinaryPath() {
  const platform = os.platform();
  const arch = os.arch();
  const key = `${platform}-${arch}`;
  const pkg = PLATFORM_PACKAGES[key];

  if (!pkg) {
    throw new Error(
      `yeager: unsupported platform ${key}. ` +
        `Supported: ${Object.keys(PLATFORM_PACKAGES).join(", ")}`
    );
  }

  try {
    // Resolve the platform-specific package's binary.
    const pkgDir = path.dirname(require.resolve(`${pkg}/package.json`));
    const binary = platform === "win32" ? "yg.exe" : "yg";
    return path.join(pkgDir, "bin", binary);
  } catch {
    throw new Error(
      `yeager: platform package ${pkg} not installed. ` +
        `Try reinstalling: npm install @yeager.sh/cli`
    );
  }
}

try {
  const binary = getBinaryPath();
  const result = execFileSync(binary, process.argv.slice(2), {
    stdio: "inherit",
    env: process.env,
  });
} catch (e) {
  if (e.status !== undefined) {
    // Child process exited with a non-zero code — propagate it.
    process.exit(e.status);
  }
  // Actual error (binary not found, etc).
  console.error(e.message);
  process.exit(1);
}
