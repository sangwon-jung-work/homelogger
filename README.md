# homelogger
bme280 temperature, humidity, pressure logger for raspberry Pi 4.

sensor value is insert to mariadb table by some interval.

## Environment

golang 1.22.3 - [Download and install - Go](https://go.dev/doc/install)

## Clone

```
$ git clone https://github.com/sangwon-jung-work/homelogger.git
$ cd homelogger/src
$ go mod tidy
$ go get -u (already downloaded, use it as needed)
```

## Check before Run or Build

src/HomeLogger.go

- const: variable wait_minute, sql_wait_second

- dbConnection: Database type, connection url. (if necessary, connection options like MaxIdleConns)

- sendLineMsg: Line notify access token. Please see this site [Line Notify](https://notify-bot.line.me/) (not required)

- main: device_name (not required)

## Install system service and Auto start on Boot

```
$ cd homelogger/src
$ go build HomeLogger.go

$ sudo nano /lib/systemd/system/homelogger.service

[Unit]
Description=BME280 sensor data logger

[Service]
Type=simple
Restart=always
RestartSec=5s
WorkingDirectory=/scvservices/script/homelogger # build file path
ExecStart=/scvservices/script/homelogger/HomeLogger # build file fullpath

[Install]
WantedBy=multi-user.target


# add execute permission
$ chmod +x /scvservices/script/homelogger/HomeLogger

# auto restart on Boot
$ sudo systemctl enable homelogger.service

# service start and check status
$ sudo systemctl start homelogger.service
$ sudo systemctl status homelogger.service
```