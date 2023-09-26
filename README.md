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

## Quick start

1. Download and install golang compiler >= 1.20.6
> https://go.dev/doc/install

2. Clone repo, install dependencie and build project

```
git clone git@github.com:dbkv9/tglog.git
cd tglog/src
go get
go build tglog.go
```

3. Create config.yaml file with your parameters

```
projects:
  project1:
    log: "/path/to/project1/log/file"
    host: "https://project1.com/"
    reportschedule: "0 * * * *"
    tgchat: -000000000
    webserver: "nginx"
    format: "$remote_addr - $remote_user [$time_local] \"$request\" $status $body_bytes_sent \"$http_referer\" \"$http_user_agent\""
  project2:
    log: "/path/to/project2/log/file"
    host: "https://project2.com/"
    reportschedule: "0 * * * *"
    tgchat: -000000000
    webserver: "nginx"
    format: "$remote_addr - $remote_user [$time_local] \"$request\" $status $body_bytes_sent \"$http_referer\" \"$http_user_agent\""
tgtoken: "6575068059:AAG6zSPg0P2qK9-KmFglVZ8q5fLYD6jeUSM"
```

This file have next structure for project configuration:
- log - path to log file
- host - full host to your website
- reportschedule - cron-like scheduler
- tgchat - id of your chat for send notification
- webserver - now nginx by default
- format - format of your log file (currently support only default NGinx access log file format)

Among other things, you need create a Telegram bot through [@botfather](https://t.me/BotFather) and set bot token to _tgtoken_ parameter.

4. Run

```
./tglog --config /path/to/config.yaml
```

## TODO
- [ ] move config file to linux system folder
- [ ] add systemd support
- [ ] create default config file automaticly
- [ ] add binaries to git release
- [ ] add Makefile for project building
