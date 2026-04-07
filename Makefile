all: build

deps:
	go install mvdan.cc/garble@latest

build:
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o support

build-unencrypt:
	make deps
	GOOS=linux GOARCH=amd64 garble -tiny -literals build -o support .

upload:
	make build && rsync -av support vpn@vpngate:~/support/ && \
	ssh vpn@vpngate sudo supervisorctl restart support && \
	rm support
