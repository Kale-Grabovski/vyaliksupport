# vyaliksupport

## Install

```
sudo apt update && sudo apt upgrade -y
sudo apt remove -y apache2
```

Install supervisor:

```
sudo apt install supervisor net-tools curl nginx -y
sudo systemctl enable supervisor
sudo systemctl start supervisor
```

Supervisor config:

```
[program:bot]
command=/home/vpn/bot/bot bot --config=/home/vpn/bot/app.yaml
directory=/home/vpn/bot
user=vpn
autostart=true
autorestart=true
startretries=5
startsecs=5
stderr_logfile=/home/vpn/bot/err.log
stdout_logfile=/home/vpn/bot/out.log
```

Reread supervisor:

```
sudo supervisorctl reread && sudo supervisorctl update
```

Nginx config for callbacks:

```
server {
    listen 1888 ssl;
    server_name domain1.com;

    ssl_certificate /etc/letsencrypt/live/domain1.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/domain1.com/privkey.pem;

    location / {
        proxy_pass http://localhost:6666;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-MerchantId $http_x_merchantId;
        proxy_set_header X-ApiKey $http_x_apiKey;
        proxy_set_header X-Secret $http_x_secret;
    }
}
```

nginx conf for tg webhook:

```
server {
    listen 8443 ssl;
    server_name domain1.com;

    ssl_certificate /etc/letsencrypt/live/domain1.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/domain1.com/privkey.pem;

    location /tg/wh {
        proxy_pass http://127.0.0.1:6677;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

Restart supervisor:

```
sudo supervisorctl reread && sudo supervisorctl update
```

## Add certs for marzban

```
sudo add-apt-repository -y ppa:certbot/certbot
sudo apt update
sudo apt install -y certbot
sudo certbot certonly --standalone -d domain1.com -d sub.domain1.com --non-interactive --agree-tos --email eie@email.com
```

copy certs to /var/lib/marzban and add them 777 rights to make it mountable at container.

```
sudo mkdir -p /var/lib/marzban/certs
sudo cp -R /etc/letsencrypt/archive/domain1.com /var/lib/marzban/certs/
sudo chmod -R 777 /var/lib/marzban/certs/
```

Important shit from /opt/marzban/.env:

```
UVICORN_HOST = "domain1.com"
UVICORN_PORT = 5443
UVICORN_SSL_CERTFILE = "/var/lib/marzban/certs/domain1.com/fullchain1.pem"
UVICORN_SSL_KEYFILE = "/var/lib/marzban/certs/domain1.com/privkey1.pem"
DASHBOARD_PATH = "/zalupa/"
SSL_CERT_FILE = /etc/ssl/certs/ca-certificates.crt
REQUESTS_CA_BUNDLE = /etc/ssl/certs/ca-certificates.crt
```

## Xray

```
cd
sudo bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install
curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh > ins.sh
chmod +x ins.sh
sudo ./ins.sh
sudo systemctl enable xray
sudo systemctl start xray
```

## Platega test curl

```
curl -X POST http://localhost:6666/callback/platega \
-H 'Content-Type: application/json' \
-H 'X-MerchantId: 87ebaee1-fe23-4514-91bb-66e3b21b16de' \
-H 'X-ApiKey: Y4TVBhwPmeegULfpqrNkEKHtJxFLqrdjn8uk1DebANeqLnnEPtyrR0qCJGYX1jD8QDr0Dok814ugZwHXGJGjqVMuEVqgYCe5bIQd' \
-d '{"id": "92b805c4-3763-4e00-9988-9267c3ae2ecb","amount": 1200, "currency": "RUB", "status": "success", "paymentMethod": 2 }'
```

## Move to new server

### Old server

Make a DB backup and copy to the new server:

```
mariadb-dump -u marzban -p marzban > dump.sql
```

Copy xray config from /var/lib/marzban/xray_config.json

### New server

Copy xray_config.json.

Stop marzban container so it doesn't use the DB.

When go to MariaDB container and recreate empty DB:

```
DROP DATABASE marzban;
CREATE DATABASE marzban CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

Run DB backup:

```
mariadb -u marzban -p marzban < dump.sql
```

Start marzban container.

## Deleting TG webhook

```
curl -X GET "https://api.telegram.org/bot<botToken>/setWebhook?remove="
```

## Support bot webhook set

```
https://api.telegram.org/bot<botToken>/setWebhook?url=https://domain1.com/api/telegram/bot&max_connections=45&drop_pending_updates=true&secret_token=secretShit
```
