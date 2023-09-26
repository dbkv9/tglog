![tglog](https://github.com/dbkv9/tglog/assets/139353879/0a70eeb9-f1ca-46c7-91c7-e9c10437bfdf)

# tglog

Realtime log analyzer for NGinx with Telegram notifications

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Codacy Badge](https://app.codacy.com/project/badge/Grade/5d0506415c1a4cbf8c08a9543e1bd4a3)](https://app.codacy.com/gh/dbkv9/tglog/dashboard?utm_source=gh&utm_medium=referral&utm_content=&utm_campaign=Badge_grade)
![tglog workflow](https://github.com/dbkv9/tglog/actions/workflows/go.yml/badge.svg)

## General features:

1. Realtime critical error notification

![image](https://github.com/dbkv9/tglog/assets/139353879/58b30490-4ca6-4df9-babf-b08535920a5f)

2. Daily reports (with cron-like scheduler for time customization)

![image](https://github.com/dbkv9/tglog/assets/139353879/8581aa5a-7b0e-496f-be07-f4afa5b33eb2)

4. Log file export to Excel sheet

![image](https://github.com/dbkv9/tglog/assets/139353879/02a6ca32-cdc0-43dc-b2fe-73367163176a)

![image](https://github.com/dbkv9/tglog/assets/139353879/fd60d6f9-13b4-4220-b734-9a2135fad39a)

## Prerequisites

For build project you must install:
- Go compiler, ver >= 1.20 https://go.dev/doc/install
- Make https://www.gnu.org/software/make/

Among other things, you need create a Telegram bot through [@botfather](https://t.me/BotFather) and set bot token to _tgtoken_ parameter in your config file.

## Quick start

1. Clone repo, build and install project
    
    ```
    git clone git@github.com:dbkv9/tglog.git
    cd tglog
    make build
    make install
    ```

2. Edit default config file in _/usr/local/etc/tglog/config.yaml_ or create new config file with custom path.

    If you want use custom config file path, you must pass it to tglog parameter:
    
    ```
    ./tglog --config /custom/path/to/config.yaml
    ```
    
    Config for example:
    ```
    projects:
      project1:
        log: "/path/to/project1/log/file"
        host: "https://project1.com/"
        reportschedule: "0 * * * *"
        tgchat: -000000000
        webserver: "nginx"
        format: "$remote_addr - $remote_user [$time_local] \"$request\" $status $body_bytes_sent \"$http_referer\" \"$http_user_agent\""
    tgtoken: "XXXXXXXXXXXXX:XXXXXXXXXXX-XXXX-XXXXXXXXXX-XXXX"
    ```

    This file have next parameters:
    - log - path to log file;
    - host - full host to your website;
    - reportschedule - cron-like scheduler;
    - tgchat - id of your chat for send notification;
    - webserver - now nginx by default;
    - format - format of your log file (currently support only default NGinx access log file format).

3. Run

    If you want startup tglog without systemd, just type:
   ```
   ./tglog
   ## or if you use custom config path
   ./tglog --config /path/to/config.yaml
   ```

   If you want use systemd, run next commands from tglog directory:

   ```
   sudo cp tglog.service /etc/systemd/system/
   sudo systemctl enable tglog.service
   sudo systemctl start tglog.service
   ```

## TODO
- [X] move config file to linux system folder
- [X] add systemd support
- [X] add binaries to git release
- [X] add Makefile for project building
