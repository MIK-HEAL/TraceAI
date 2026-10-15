const fs = require("fs");
const fsp = require("fs/promises");
const http = require("http");
const https = require("https");
const os = require("os");
const path = require("path");
const { pipeline } = require("stream/promises");

const pkg = require("../package.json");

const DEFAULT_BASE_URL = "https://github.com/MIK-HEAL/TraceAI/releases/download";

function currentVersion() {
  return process.env.TRACEAI_VERSION || pkg.version;
}

function normalizedPlatform(platform) {
  switch (platform) {
    case "win32":
      return "windows";
    case "darwin":
      return "darwin";
    default:
      return "linux";
  }
}

function normalizedArch(arch) {
  switch (arch) {
    case "x64":
      return "amd64";
    case "arm64":
      return "arm64";
    default:
      return arch;
  }
}

function binaryName() {
  return process.platform === "win32" ? "traceai.exe" : "traceai";
}

function assetName() {
  return `traceai-${normalizedPlatform(process.platform)}-${normalizedArch(process.arch)}${process.platform === "win32" ? ".exe" : ""}`;
}

function releaseBaseUrl() {
  return process.env.TRACEAI_RELEASE_BASE_URL || DEFAULT_BASE_URL;
}

function binaryUrl() {
  if (process.env.TRACEAI_BINARY_URL) {
    return process.env.TRACEAI_BINARY_URL;
  }
  return `${releaseBaseUrl()}/v${currentVersion()}/${assetName()}`;
}

function cacheDir() {
  return process.env.TRACEAI_CACHE_DIR || path.join(os.homedir(), ".traceai");
}

function binaryPath() {
  return path.join(cacheDir(), "bin", currentVersion(), binaryName());
}

async function ensureBinary() {
  const target = binaryPath();
  try {
    await fsp.access(target, fs.constants.X_OK);
    return target;
  } catch (_) {
    // download below
  }

  await fsp.mkdir(path.dirname(target), { recursive: true });
  const tmp = `${target}.download`;
  await download(binaryUrl(), tmp);
  if (process.platform !== "win32") {
    await fsp.chmod(tmp, 0o755);
  }
  try {
    await fsp.rename(tmp, target);
  } catch (err) {
    await fsp.rm(target, { force: true });
    await fsp.rename(tmp, target);
  }
  return target;
}

async function download(url, destination) {
  await new Promise((resolve, reject) => {
    const client = url.startsWith("https:") ? https : http;
    const request = client.get(url, (response) => {
      const status = response.statusCode || 0;
      if ([301, 302, 303, 307, 308].includes(status) && response.headers.location) {
        response.resume();
        const next = new URL(response.headers.location, url).toString();
        download(next, destination).then(resolve, reject);
        return;
      }
      if (status !== 200) {
        response.resume();
        reject(new Error(`download failed with status ${status} from ${url}`));
        return;
      }
      const file = fs.createWriteStream(destination);
      pipeline(response, file).then(resolve, reject);
    });
    request.on("error", reject);
  });
}

async function runTraceAI(argv) {
  const binary = await ensureBinary();
  const child = require("child_process").spawn(binary, argv, {
    stdio: "inherit",
  });
  child.on("exit", (code, signal) => {
    if (signal) {
      process.kill(process.pid, signal);
      return;
    }
    process.exit(code ?? 0);
  });
  child.on("error", (err) => {
    console.error(err.message);
    process.exit(1);
  });
}

module.exports = {
  assetName,
  binaryName,
  binaryPath,
  binaryUrl,
  currentVersion,
  ensureBinary,
  runTraceAI,
};
