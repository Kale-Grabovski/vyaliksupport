all: build

build:
	GOOS=linux GOARCH=amd64 go build -o support

upload:
	make build && rsync -av support vpn@vpnnl1:~/support/ && \
	ssh vpn@vpnnl1 sudo supervisorctl restart support && \
	rm bot
