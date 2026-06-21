# TraceAI Release

> 对外发布和安装入口。

## 快速安装

```bash
go install github.com/MIK-HEAL/TraceAI/cmd/traceai@latest
```

## 锁定版本

```bash
go install github.com/MIK-HEAL/TraceAI/cmd/traceai@v0.1.0-beta.1
```

## 本地构建

```bash
go build -o bin/traceai ./cmd/traceai
```

## 运行验证

```bash
./bin/traceai version
./bin/traceai health
./bin/traceai metrics --format json
```

Windows:

```powershell
.\bin\traceai.exe version
```
