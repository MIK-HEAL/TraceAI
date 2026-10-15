#!/usr/bin/env node

const { ensureBinary } = require("../lib/install");

async function main() {
  try {
    await ensureBinary();
  } catch (err) {
    console.warn(`[traceai] postinstall skipped binary download: ${err.message}`);
  }
}

main().catch((err) => {
  console.warn(`[traceai] postinstall warning: ${err.message}`);
});
