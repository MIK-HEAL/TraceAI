# TraceAI npm package

This wrapper installs the `traceai` command and downloads the official TraceAI binary on demand.

The binary is downloaded from the GitHub Release with the same version as this package. Release assets use the names `traceai-linux-amd64`, `traceai-darwin-arm64`, and so on (Windows assets end in `.exe`). Set `TRACEAI_BINARY_URL` or `TRACEAI_RELEASE_BASE_URL` when using a private mirror.

[GitHub](https://github.com/MIK-HEAL/TraceAI) · `npm install -g @clonap4d/traceai`
