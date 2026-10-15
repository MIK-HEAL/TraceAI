#!/usr/bin/env node

const { runTraceAI } = require("../lib/install");

runTraceAI(process.argv.slice(2)).catch((err) => {
  console.error(`[traceai] ${err.message}`);
  process.exit(1);
});
