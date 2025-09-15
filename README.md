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
[program:support]
command=/home/vpn/support/support support --config=/home/vpn/support/app.yaml
directory=/home/vpn/support
user=vpn
autostart=true
autorestart=true
startretries=5
startsecs=5
stderr_logfile=/home/vpn/support/err.log
stdout_logfile=/home/vpn/support/out.log
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

## Add certs

```
sudo add-apt-repository -y ppa:certbot/certbot
sudo apt update
sudo apt install -y certbot
sudo certbot certonly --standalone -d domain1.com --non-interactive --agree-tos --email eie@email.com
```

## Deleting TG webhook

```
curl -X GET "https://api.telegram.org/bot<botToken>/setWebhook?remove="
```

## Support bot webhook set

```
https://api.telegram.org/bot<botToken>/setWebhook?url=https://domain1.com/api/telegram/bot&max_connections=45&drop_pending_updates=true&secret_token=secretShit
```
