# nginx setup for production

These site configs are intended for the production VPS. nginx is the only public-facing service; Docker publishes Merio and Temporal UI on localhost-only ports.

## One-time VPS steps

### 1. Generate Cloudflare origin certificate

Cloudflare -> SSL/TLS -> Origin Server -> Create Certificate.

- Hostnames: `*.merio.io` and `merio.io`
- Validity: 15 years
- Save the certificate and private key.

### 2. Place certs on VPS

```bash
sudo mkdir -p /etc/ssl/cloudflare
sudo nano /etc/ssl/cloudflare/merio.io.pem
sudo nano /etc/ssl/cloudflare/merio.io.key
sudo chmod 0600 /etc/ssl/cloudflare/*
sudo chown root:root /etc/ssl/cloudflare/*
```

### 3. Install nginx

```bash
sudo apt install -y nginx
```

### 4. Deploy site configs

From `/opt/merio`:

```bash
sudo cp deploy/nginx/merio.io.conf /etc/nginx/sites-available/merio.io
sudo cp deploy/nginx/temporal.merio.io.conf /etc/nginx/sites-available/temporal.merio.io
sudo cp deploy/nginx/docs.merio.io.conf /etc/nginx/sites-available/docs.merio.io
sudo cp deploy/nginx/dev.merio.io.conf /etc/nginx/sites-available/dev.merio.io

sudo ln -sf /etc/nginx/sites-available/merio.io /etc/nginx/sites-enabled/
sudo ln -sf /etc/nginx/sites-available/temporal.merio.io /etc/nginx/sites-enabled/
sudo ln -sf /etc/nginx/sites-available/docs.merio.io /etc/nginx/sites-enabled/
sudo ln -sf /etc/nginx/sites-available/dev.merio.io /etc/nginx/sites-enabled/
```

### 5. Set up Temporal basic auth

```bash
sudo apt install -y apache2-utils
sudo mkdir -p /etc/nginx/htpasswd
sudo htpasswd -c /etc/nginx/htpasswd/temporal admin
sudo chmod 0640 /etc/nginx/htpasswd/temporal
sudo chown root:www-data /etc/nginx/htpasswd/temporal
```

### 6. Test and reload

```bash
sudo nginx -t
sudo systemctl reload nginx
```

### 7. Cloudflare settings

In Cloudflare -> SSL/TLS:

- Mode: Full (strict)
- Always Use HTTPS: on
- Automatic HTTPS Rewrites: on
- Min TLS Version: 1.2

In Cloudflare -> DNS, verify:

- `merio.io` -> A -> `<vps-ip>` (orange cloud)
- `temporal.merio.io` -> A -> `<vps-ip>` (orange cloud)
- `docs.merio.io` -> A -> `<vps-ip>` (orange cloud)
- `dev.merio.io` -> A -> `<vps-ip>` (orange cloud)

## Verify after setup

```bash
curl -k https://localhost --resolve merio.io:443:127.0.0.1 -H "Host: merio.io"
curl -I https://merio.io
```

Before the Docker stack is running, nginx may return a proxy error for `merio.io`. After the stack is up, it should serve the app.
