const fs = require("fs");
const fsp = require("fs/promises");
const http = require("http");
const https = require("https");
const os = require("os");
const path = require("path");
const { pipeline } = require("stream/promises");
const crypto = require("crypto");

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

function checksumUrl() {
  if (process.env.TRACEAI_CHECKSUM_URL) return process.env.TRACEAI_CHECKSUM_URL;
  return `${releaseBaseUrl()}/v${currentVersion()}/SHA256SUMS`;
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
    const marker = await fsp.readFile(`${target}.sha256`, "utf8");
    if ((await sha256File(target)) === marker.trim().toLowerCase()) return target;
    await fsp.rm(target, { force: true });
  } catch (_) {
    // download below
  }

  await fsp.mkdir(path.dirname(target), { recursive: true });
  const tmp = `${target}.download`;
  const checksumTmp = `${tmp}.sha256`;
  let expected = "";
  try {
    await download(binaryUrl(), tmp);
    await download(checksumUrl(), checksumTmp);
    const manifest = await fsp.readFile(checksumTmp, "utf8");
    expected = checksumForAsset(manifest, assetName());
    if (!expected) throw new Error(`checksum for ${assetName()} not found in SHA256SUMS`);
    const actual = await sha256File(tmp);
    if (actual !== expected) throw new Error(`SHA256 mismatch for ${assetName()}`);
  } catch (err) {
    await fsp.rm(tmp, { force: true });
    await fsp.rm(checksumTmp, { force: true });
    throw err;
  }
  await fsp.rm(checksumTmp, { force: true });
  if (process.platform !== "win32") {
    await fsp.chmod(tmp, 0o755);
  }
  try {
    await fsp.rename(tmp, target);
  } catch (err) {
    await fsp.rm(target, { force: true });
    await fsp.rename(tmp, target);
  }
  await fsp.writeFile(`${target}.sha256`, expected, "utf8");
  return target;
}

function checksumForAsset(manifest, name) {
  for (const line of manifest.split(/\r?\n/)) {
    const match = line.trim().match(/^([a-fA-F0-9]{64})\s+\*?(.+)$/);
    if (match && path.basename(match[2].trim()) === name) return match[1].toLowerCase();
  }
  return "";
}

async function sha256File(file) {
  const hash = crypto.createHash("sha256");
  await new Promise((resolve, reject) => {
    const stream = fs.createReadStream(file);
    stream.on("data", chunk => hash.update(chunk));
    stream.on("error", reject);
    stream.on("end", resolve);
  });
  return hash.digest("hex");
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
  checksumUrl,
  checksumForAsset,
  currentVersion,
  ensureBinary,
  runTraceAI,
};
