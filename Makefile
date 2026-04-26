all: build

build:
	GOOS=linux GOARCH=amd64 go build -o bot

upload:
	make build && rsync -av -e "ssh -p 14888" Dockerfile docker-compose.yaml bot vpn@vpnbro:~/support/ && \
	ssh -p 14888 vpn@vpnbro "cd ~/support && sudo docker-compose up -d --build && sudo docker image prune -f" && \
	rm bot
