# ghrunner

GitHub Actions Self-Hosted Runner 管理工具。

## 安裝

```shell
go install github.com/shared-utils/ghrunner@latest
```

## 命令

| 命令 | 說明 |
|------|------|
| `setup` | 下載並配置 runners |
| `enable` | 建立系統服務（macOS: LaunchAgent, Linux: systemd） |
| `disable` | 刪除系統服務 |
| `start` | 啟動 runners |
| `stop` | 停止服務 |

## 使用

### 1. 設定 Runners

```shell
ghrunner setup \
  --github-token=YOUR_TOKEN \
  --orgs=org1,org2 \
  --runners-per-org=2 \
  --additional-labels=self-hosted,linux
```

### 2. 建立系統服務

```shell
# macOS
ghrunner enable

# Linux (需要 root)
sudo ghrunner enable
```

### 3. 啟動/停止

**透過服務管理：**
```shell
# macOS
launchctl load ~/Library/LaunchAgents/com.github.actions.runner.plist
launchctl unload ~/Library/LaunchAgents/com.github.actions.runner.plist

# Linux
sudo systemctl start ghrunner-<org>
sudo systemctl stop ghrunner-<org>
```

**直接執行（前台）：**
```shell
ghrunner start
# Ctrl+C 優雅停止
```

## 目錄結構

```
~/.github-runners/
├── org1/
│   ├── hostname-1/
│   └── hostname-2/
└── org2/
    ├── hostname-1/
    └── hostname-2/
```

## 環境變數

| 變數 | 說明 | 預設值 |
|------|------|--------|
| `GITHUB_TOKEN` | GitHub PAT | - |
| `ROOT_RUNNERS_DIR` | Runner 根目錄 | `~/.github-runners` |
