
## Installation
```shell
go install github.com/shared-utils/ghrunner@latest
```

## Linux Systemd

Set the root directory where your runners are located:
```shell
export ROOT_RUNNERS_DIR=/github-runners
```

Create systemd service file and reload daemon:
```shell
sudo tee /etc/systemd/system/ghrunner.service > /dev/null <<EOF
[Unit]
Description=GitHub Actions Runner Manager
After=network.target

[Service]
Type=simple
WorkingDirectory=$ROOT_RUNNERS_DIR
ExecStart=$(which ghrunner)
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
```

Manage the service:
```shell
sudo systemctl enable ghrunner  # Enable auto-start on boot
sudo systemctl start ghrunner   # Start service
sudo systemctl stop ghrunner    # Stop service
sudo systemctl restart ghrunner # Restart service
```

## MacOS LaunchAgents

Set the root directory where your runners are located:
```shell
export ROOT_RUNNERS_DIR=~/github-runners
```

Create launchd plist file (runs as current user with login shell environment):
```shell
cat > ~/Library/LaunchAgents/com.github.ghrunner.plist <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.github.ghrunner</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/zsh</string>
        <string>-l</string>
        <string>-c</string>
        <string>source ~/.zshrc && exec $(which ghrunner)</string>
    </array>
    <key>WorkingDirectory</key>
    <string>$ROOT_RUNNERS_DIR</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/ghrunner.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/ghrunner.error.log</string>
</dict>
</plist>
EOF
```

Manage the service:
```shell
launchctl load ~/Library/LaunchAgents/com.github.ghrunner.plist   # Start service
launchctl unload ~/Library/LaunchAgents/com.github.ghrunner.plist # Stop service
```