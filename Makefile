all: build

deps:
	go install mvdan.cc/garble@latest

build-unencrypt:
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o support

build:
	make deps
	GOOS=linux GOARCH=amd64 garble -tiny -literals build -o support .

upload-old:
	make build && rsync -av support vpn@vpngate:~/support-old/ && \
	ssh vpn@vpngate sudo supervisorctl restart support-old && \
	rm support

upload:
	make build && rsync -av support vpn@vpngate:~/support/ && \
	rsync -av support vpn@vpngate:~/support-old/ && \
	ssh vpn@vpngate sudo supervisorctl restart {support,support-old} && \
	rm support
