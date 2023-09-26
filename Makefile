BINARY_NAME=tglog

clean:
	go clean
	rm -rf /tmp/${BINARY_NAME}

build:
	go build -o /tmp/${BINARY_NAME} src/tglog.go

install:
	mv /tmp/${BINARY_NAME} /usr/local/bin 
	mkdir -p /usr/local/etc/tglog/
	@ echo ''
	@ echo '[OK] Now, create config file in "/user/local/etc/tglog/config.yaml", for example see config.example.yaml'
	@ echo '     or use --config arg for set config path manualy'
