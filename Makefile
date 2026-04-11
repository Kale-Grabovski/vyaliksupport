all: build

deps:
	go install mvdan.cc/garble@latest

build-encrypt:
	make deps
	GOOS=linux GOARCH=amd64 garble -tiny -literals build -o support .

build:
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bot && cp bot support

upload:
	make build && rsync -av -e "ssh -p 14888" bot vpn@xvpnnl:~/bot/ && \
	ssh -p 14888 vpn@xvpnnl sudo supervisorctl restart {notify,group} && \
	rsync -av -e "ssh -p 14888" support vpn@vpngate:~/sup/ && \
	ssh-p 14888 vpn@vpngate sudo supervisorctl restart sup && \
	rm {bot,support}

upload-prod:
	make build && rsync -av -e "ssh -p 14888" bot vpn@xvpnnl:~/bot/ && \
	rsync -av -e "ssh -p 14888" support vpn@vpngate:~/support/ && \
	ssh -p 14888 vpn@xvpnnl sudo supervisorctl restart {notify,group} && \
	ssh -p 14888 vpn@vpngate sudo supervisorctl restart support  && \
	rm {bot,support}
