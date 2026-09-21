# TLS avec Let's Encrypt

Ce document remplace l'ancienne procédure fondée sur l'adresse IP brute. En
production, l'API RelaisDesk doit être appelée uniquement par :

```text
https://api.relaisdesk.fr
```

Le DNS `A`/`AAAA` de ce nom doit pointer vers le serveur Oracle. Le certificat
Oracle doit contenir `api.relaisdesk.fr` et, pendant la transition,
`api.informatiqueadomicile03.fr` dans ses SAN. L'ancien nom reste ainsi un alias
HTTPS fonctionnel pour les POST des launchers déjà distribués.

Les DNS `A` de `relaisdesk.fr` et `www.relaisdesk.fr` pointent vers
l'hébergement OVH. Leur certificat est géré par OVH. Aucun bloc Nginx Oracle
ne doit déclarer ces deux noms.

## Certificat

Exemple Certbot avec le greffon du serveur web déjà en place :

```bash
sudo certbot certonly --nginx --cert-name api.relaisdesk.fr \
  -d api.relaisdesk.fr \
  -d api.informatiqueadomicile03.fr
```

Ou, si aucun serveur n'écoute temporairement sur le port 80 :

```bash
sudo certbot certonly --standalone --cert-name api.relaisdesk.fr \
  -d api.relaisdesk.fr \
  -d api.informatiqueadomicile03.fr
```

Les chemins obtenus sont généralement :

```text
/etc/letsencrypt/live/api.relaisdesk.fr/fullchain.pem
/etc/letsencrypt/live/api.relaisdesk.fr/privkey.pem
```

L'API peut utiliser directement ces fichiers via `TLS_CERT`/`TLS_KEY`, ou être
placée derrière un frontal HTTPS local. Le frontal ne doit transmettre les
en-têtes `X-Forwarded-*` qu'après les avoir remplacés, et l'API ne doit pas être
exposée publiquement en HTTP.

## Configuration API minimale

```dotenv
API_PORT=8443
API_BIND=127.0.0.1
SERVER_IP=api.relaisdesk.fr
RUSTDESK_RENDEZVOUS_PORT=21116
RUSTDESK_RELAY_PORT=21117
TLS_CERT=/opt/relaisdesk/certs/fullchain.pem
TLS_KEY=/opt/relaisdesk/certs/privkey.pem
DEV_HTTP=false
```

Les clients sont compilés avec l'URL HTTPS ci-dessus. RustDesk Community joint
directement `hbbs`/`hbbr` ; aucun proxy SOCKS5 ni composant RustDesk Server Pro
n'est utilisé. L'option `api-server` générée par les launchers désigne ici
uniquement l'API RelaisDesk consommée par le fork.

## Vérifications avant production

```bash
curl --fail --show-error https://api.relaisdesk.fr/api/v1/health
curl --fail --show-error https://api.informatiqueadomicile03.fr/api/v1/health
openssl s_client -connect api.relaisdesk.fr:443 \
  -servername api.relaisdesk.fr </dev/null
sudo certbot renew --dry-run
```

La préproduction `relaist.cluster129.hosting.ovh.net` doit disposer de son
propre certificat correspondant exactement à ce nom si elle reste accessible
en HTTPS. Elle doit également conserver l'en-tête `X-Robots-Tag: noindex`.
