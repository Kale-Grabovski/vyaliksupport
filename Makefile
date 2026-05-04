all: build

deps:
	go install mvdan.cc/garble@latest

build:
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o support

build-unencrypt:
	make deps
	GOOS=linux GOARCH=amd64 garble -tiny -literals build -o support .

upload:
	make build && rsync -av -e "ssh -p 14888" support vpn@vpngate:~/support/ && \
	ssh -p 14888 vpn@vpngate sudo supervisorctl restart support && \
	rm support
