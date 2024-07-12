# Caldav notification daemon
![изображение](https://github.com/user-attachments/assets/05545d09-d723-421b-8296-b770ad3fb471)

### prepare
```
pip install -r requirements.txt
sudo ln -s $PWD/caldav-fetch.py /usr/bin/caldav-fetch.py
go mod tidy
```

#### run
```
go run main.go & disown
```

#### or build and run
```
go build -o caldav_daemon .
./caldav_daemon & disown
```

#### to stop
```
killall caldav_daemon
```

###### or
```
killall go
```

## environment variables to configure
### must be set
- CALDAV_USERNAME
- CALDAV_PASSWORD
- CALDAV_URL

### optional
- CALDAV_NOTIFY_ICON
- CALDAV_SERVER_OFFSET_HOURS default - same as server
- CALDAV_REFRESH_PERIOD_MINUTES default 10
- CALDAV_NOTIFY_BEFORE_MINUTES default 5

## for better experience
it is useful to create env file with environment variables and source it
