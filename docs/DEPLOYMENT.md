# Deploiement

## Prerequis

- Ubuntu Server sur le VPS Oracle.
- Une version de Go compatible avec la directive de `api/go.mod` (actuellement
  Go 1.26.6).
- SQLite via `modernc.org/sqlite` (pas de CGO requis pour la base).
- Nom de domaine + certificat TLS pour l'API en production.
- Nginx Oracle limité au nom `api.relaisdesk.fr`, sur `443`, transmettant vers
  l'API locale `127.0.0.1:8443` ; le port
  `8443` ne doit pas être ouvert publiquement.
- Le site `relaisdesk.fr` et `www.relaisdesk.fr`, leurs fichiers et leur
  certificat restent exclusivement sur l'hébergement web OVH.
- Docker/Podman ou une chaîne Rust pour construire les forks `hbbs`/`hbbr`.
- Une paire Ed25519 d'autorisation distincte de `id_ed25519`.
- Une paire Ed25519 de signature des versions, dont seule la clé publique est
  copiée sur le serveur.

## Clé d'autorisation

Créer la paire avant de démarrer l'API :

```bash
cd api
sudo install -d -m 0700 -o relaisdesk -g relaisdesk /etc/relaisdesk
go run ./cmd/network-keygen -out /tmp/network-auth-ed25519
sudo install -m 0600 -o relaisdesk -g relaisdesk /tmp/network-auth-ed25519 /etc/relaisdesk/network-auth-ed25519
rm /tmp/network-auth-ed25519
```

Reporter la clé publique affichée dans `rustdesk-server/.env` :

```text
RELAISDESK_AUTH_PUBLIC_KEYS=relaisdesk-1=<cle-publique-base64url>
```

Le fichier privé est référencé par `NETWORK_AUTH_PRIVATE_KEY_FILE` dans
`/etc/relaisdesk/api.env`. Voir `FORK_AUTHORIZATION.md` pour la rotation.

Configurer aussi `RELEASE_MANIFEST_PATH` et `RELEASE_PUBLIC_KEY` selon
`RELEASE_SIGNING.md`. La clé privée de version ne doit jamais être copiée sur
le serveur.

## Installation serveur

```bash
sudo ./scripts/setup_server.sh
sudo ./scripts/deploy_api.sh
sudo ./scripts/firewall.sh
```

Avant d'accepter un paiement réel, suivre intégralement
`docs/LEGAL_RELEASE_CHECKLIST.md`. Le déploiement installe une rotation des
fichiers de `/var/log/relaisdesk` limitée à 365 jours. Configurer également
`systemd-journald` avec une rétention maximale de douze mois si les sorties des
services y sont conservées.

Configurer `PUBLIC_WEBSITE_URL` avec l'origine réellement servie aux clients :
`https://relaist.cluster129.hosting.ovh.net` pendant la préproduction, puis
`https://relaisdesk.fr` après la mise en production du domaine. Cette valeur est
utilisée dans les liens de connexion et les relances. Les relances automatiques
nécessitent également une configuration SMTP fonctionnelle ; leur traitement
est lancé au démarrage de l'API puis toutes les heures. Les événements Stripe
et les courriels sont déposés dans une file SQLite persistante et repris après
un redémarrage par un unique worker local ; aucun Redis, RabbitMQ, agent de
télémétrie ni service de supervision supplémentaire n'est requis.

## Authentification des courriels

Demander au prestataire SMTP les valeurs exactes à publier pour
`relaisdesk.fr`, puis configurer dans sa zone DNS :

- un seul enregistrement SPF autorisant le prestataire (ne pas publier deux
  enregistrements SPF concurrents) ;
- la clé ou le CNAME DKIM sous `<selecteur>._domainkey.relaisdesk.fr` ;
- `_dmarc.relaisdesk.fr`, d'abord contrôlé en préproduction, puis avec
  `p=quarantine` ou `p=reject` avant la production.

Renseigner ensuite dans `/etc/relaisdesk/api.env` :

```text
SMTP_FROM=RelaisDesk <contact@relaisdesk.fr>
EMAIL_DOMAIN=relaisdesk.fr
SMTP_DKIM_SELECTOR=<selecteur-fourni-par-le-prestataire>
EMAIL_REQUIRE_DNS_AUTH=true
```

L'API vérifie seulement la cohérence locale du domaine et du sélecteur au
démarrage afin de rester légère. La vérification DNS est une commande ponctuelle :

```bash
/opt/relaisdesk/api/relaisdesk-emailcheck \
  -domain relaisdesk.fr -selector <selecteur-fourni-par-le-prestataire> -strict=true
```

Réaliser aussi un envoi réel vers au moins deux fournisseurs de messagerie et
contrôler dans les en-têtes reçus les résultats `spf=pass`, `dkim=pass` et
`dmarc=pass`. Le programme ajoute `Date`, `Message-ID`, `Reply-To` et
`Auto-Submitted`, mais la signature DKIM elle-même reste effectuée par le
prestataire SMTP.

## Sauvegardes

`deploy_api.sh` installe et active `relaisdesk-backup.timer`, génère une clé
locale si elle n'existe pas et conserve 30 jours de sauvegardes chiffrées dans
`/var/backups/relaisdesk`. Après le premier déploiement :

1. copier `/etc/relaisdesk/backup.key` vers un coffre hors ligne ;
2. monter si possible un stockage hors machine sur
   `/mnt/relaisdesk-backups` et le déclarer dans
   `/etc/relaisdesk/backup.env` ;
3. lancer une sauvegarde puis un test de restauration selon `docs/ADMIN.md`.

La clé ne doit jamais être stockée avec l'unique exemplaire des sauvegardes.

Pour une copie distante **Google Drive**, sans montage de disque, suivre le
[guide dédié](guides/SAUVEGARDE_GOOGLE_DRIVE.md). Il ajoute un service indépendant,
avec autorisation OAuth limitée, envoi chiffré et relecture de contrôle. Ne pas
activer son minuteur avant le premier test distant réussi et la conservation
séparée de la clé ; ne pas laisser tourner en parallèle l'ancien minuteur local.

Construire et démarrer les serveurs réseau forkés :

```bash
cd rustdesk-server
install -d -m 0700 data
docker compose build --pull
docker compose up -d
```

## Services

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now relaisdesk-api
sudo systemctl enable --now relaisdesk-backup.timer
```

`hbbs` et `hbbr` sont gérés exclusivement par le Compose ci-dessus
(`restart: unless-stopped`). Ne pas activer en parallèle les anciennes unités
systemd : deux instances se disputeraient les ports et une unité non configurée
pourrait démarrer sans les clés publiques d'autorisation.

## Verification

```bash
systemctl status relaisdesk-api
systemctl status relaisdesk-backup.timer
curl https://api.relaisdesk.fr/api/v1/health
curl https://api.relaisdesk.fr/api/v1/public/releases/latest
docker compose -f rustdesk-server/docker-compose.yml ps
docker compose -f rustdesk-server/docker-compose.yml logs --tail=100 hbbs hbbr
ss -tlnp
ss -ulnp
```

Ports publics attendus :

- TCP `443` : API HTTPS RelaisDesk (`https://api.relaisdesk.fr`)
- TCP `21115`, `21116`, `21117` : RustDesk Community
- UDP `21116` : perforation NAT RustDesk
- TCP `22` : SSH
- TCP `80`, `443` : HTTP / HTTPS de l'API et challenges Let's Encrypt ; le
  site principal n'est pas servi par Oracle.

Le frontal doit remplacer les en-têtes `X-Forwarded-For` et `X-Real-IP` reçus
du client. L'amont API reste protégé par TLS et utilise le certificat de
`api.relaisdesk.fr`; configurer le SNI de l'amont si le frontal
joint `127.0.0.1:8443`.

Les ports WebSocket `21118-21119/tcp` peuvent rester fermés si aucun client Web
n'est utilisé. Le port historique `1080/tcp` doit être fermé.

Le Compose du fork ne publie pas `21118-21119`. Si un client Web est ajouté
ultérieurement, placer ces ports derrière un frontal inaccessible directement
et définir sur `hbbs`/`hbbr` la liste exacte de ses adresses :

```text
RELAISDESK_TRUSTED_WS_PROXY_IPS=127.0.0.1,172.20.0.10
```

Sans cette variable, les en-têtes `X-Real-IP` et `X-Forwarded-For` des
connexions WebSocket sont ignorés. Préparer aussi le volume avant le premier
démarrage des conteneurs non privilégiés :

```bash
install -d -m 0700 rustdesk-server/data
chown 10001:10001 rustdesk-server/data
```

Ne pas activer le mode obligatoire avant d'avoir distribué le client forké : un
client RustDesk amont ne sait pas produire les preuves d'autorisation. La
recette minimale doit durer plus de cinq minutes afin de vérifier le
renouvellement du jeton et le canal de santé.

Depuis le poste Windows de construction, la recette automatisée complète se
lance avec :

```powershell
.\scripts\preproduction-check.ps1
.\scripts\preproduction-check.ps1 -DKIMSelector <selecteur-dkim>
.\scripts\preproduction-check.ps1 -LiveApiUrl https://api.relaisdesk.fr
```
