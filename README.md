
## Installation
```shell
go install github.com/shared-utils/ghrunner@latest
```

## Linux Systemd

(Optional) Create a dedicated runner user:
```shell
export RUNNER_USER=ghrunner
sudo adduser --disabled-password --gecos "" $RUNNER_USER
```

Or use current user:
```shell
export RUNNER_USER=$(whoami)
```

Set the root directory:
```shell
export ROOT_RUNNERS_DIR=/home/$RUNNER_USER/github-runners
```

Create wrapper script to load user environment variables:
```shell
sudo tee /home/$RUNNER_USER/ghrunner-wrapper.sh > /dev/null <<'SCRIPT'
#!/bin/bash
source ~/.profile 2>/dev/null
source ~/.bashrc 2>/dev/null
exec ghrunner
SCRIPT
sudo chmod +x /home/$RUNNER_USER/ghrunner-wrapper.sh
sudo chown $RUNNER_USER:$RUNNER_USER /home/$RUNNER_USER/ghrunner-wrapper.sh
```

Fix ownership of runner files (after copying runner files):
```shell
sudo chown -R $RUNNER_USER:$RUNNER_USER $ROOT_RUNNERS_DIR
```

Create systemd service file (runs as specified user with full environment):
```shell
sudo tee /etc/systemd/system/ghrunner-$RUNNER_USER.service > /dev/null <<EOF
[Unit]
Description=GitHub Actions Runner Manager ($RUNNER_USER)
After=network.target

[Service]
Type=simple
User=$RUNNER_USER
Group=$RUNNER_USER
WorkingDirectory=$ROOT_RUNNERS_DIR
ExecStart=/home/$RUNNER_USER/ghrunner-wrapper.sh
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
```

Manage the service:
```shell
sudo systemctl enable ghrunner-$RUNNER_USER  # Enable auto-start on boot
sudo systemctl start ghrunner-$RUNNER_USER   # Start service
sudo systemctl stop ghrunner-$RUNNER_USER    # Stop service
sudo systemctl restart ghrunner-$RUNNER_USER # Restart service
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