all: build

build:
	GOOS=linux GOARCH=amd64 go build -o support

upload:
	make build && rsync -av support vpn@vpngate:~/support/ && \
	ssh vpn@vpngate sudo supervisorctl restart support && \
	rm support
