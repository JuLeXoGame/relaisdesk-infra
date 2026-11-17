# Durcissement du serveur Oracle - plan du 2026-11-17

Statut : **préparé, non exécuté**. Tous les relevés ci-dessous ont été faits en lecture seule sur `79.72.27.213`. Le script `scripts/hardening-oracle-20261117.sh` doit être copié sur le serveur et lancé en root, une sous-commande à la fois, après validation de l'exploitant.

## 1. Constat (relevés du 2026-11-17)

### Événement notable pendant l'audit
Le serveur a **redémarré à 17:05 UTC** (hors de cet audit) : noyau passé de 6.8.0-1062 à **6.8.0-1067-oracle**. Au retour, tous les services (API, nginx, Docker, wg0, php8.1-fpm, MariaDB) et les deux conteneurs étaient actifs, les deux domaines répondaient 200. Le redémarrage « noyau + libc » n'est donc plus à faire ; ce reboot a servi de test de reprise.

### Domaine historique `api.informatiqueadomicile03.fr`
- Résolu en DNS vers la même IP ; vhost proxifiant tout vers l'API (aucune des protections du vhost `relaisdesk-api`).
- Le certificat utilisé (`live/informatiqueadomicile03.fr`) couvre bien `api.informatiqueadomicile03.fr` ; expire le 2027-01-15.
- **Usage réel non mesurable aujourd'hui** : le journal nginx global n'enregistre pas le nom d'hôte, le vhost n'a pas de journal propre et l'API ne journalise ni Host ni Origin. Ordre de grandeur du trafic total (tous vhosts) : environ 42 000 lignes/jour, dont 1 918 `POST /api/v1/devices/heartbeat` et 511 `/api/v1/health` sur la veille ; une partie vient de `79.72.27.213` lui-même (supervision/PHP). Rien ne permet d'attribuer ce trafic à l'ancien domaine.
- Le script ajoute un journal dédié `/var/log/nginx/api-legacy.access.log` : après quelques jours, `wc -l` donne l'usage réel et permet de décider de la suppression du vhost.
- Constaté : l'en-tête `Server: nginx/1.18.0 (Ubuntu)` fuit la version sur ce vhost seulement.
- CORS de l'API autorise toujours `https://informatiqueadomicile03.fr` (code API non modifié).

### Redémarrage / persistance (tout revient seul)
| Élément | État |
|---|---|
| `rustdesk-hbbs`, `rustdesk-hbbr` | restart policy `unless-stopped` |
| `relaisdesk-api`, `nginx`, `docker`, `wg-quick@wg0`, `php8.1-fpm`, `mariadb`, `netfilter-persistent` | `enabled` |
| Authentification hbbs/hbbr | `RELAISDESK_AUTH_REQUIRED=Y` et clé publique présentes dans les deux conteneurs |

### iptables
- Persistance : paquet `netfilter-persistent`/`iptables-persistent`, fichier **`/etc/iptables/rules.v4`** (root:root 640, dernière écriture 2 sept.), rechargé au boot. `ufw` est inactif (`ENABLED=no`).
- Cause des doublons : `rules.v4` contient déjà **3** règles `51820/udp` **et** `wg-quick@wg0` (PostUp) en insère une de plus à chaque démarrage (`iptables -I INPUT ...`), d'où 4. Même mécanisme pour `nat POSTROUTING -o ens3 -j MASQUERADE` (4 exemplaires, hors périmètre ici).
- Conséquence : dédoublonner uniquement en live serait annulé au prochain boot. Le script ne conserve donc qu'**une** règle 51820 dans le **fichier** et une seule en live. Garder une copie persistante évite que l'ouverture du port WireGuard dépende de l'ordre de démarrage entre `netfilter-persistent` et `wg-quick` ; après un redémarrage, deux règles coexistent (fichier + PostUp), sans conséquence.

### Certbot
- Deux certbot : apt 1.21 (`certbot.timer`, `/usr/bin/certbot`) et snap 5.10 (`snap.certbot.renew.timer`). La ligne cron root `0 4 * * * certbot renew ... --deploy-hook ...` résout `certbot` vers **`/usr/bin/certbot` (apt)** car `/usr/bin` précède `/snap/bin` dans le PATH. Trois mécanismes renouvellent donc en parallèle.
- Le hook `/etc/letsencrypt/renewal-hooks/deploy/relaisdesk.sh` (copie vers `/opt/relaisdesk/certs`, redémarre l'API, recharge nginx) s'exécute **automatiquement via le répertoire de hooks** pour tout renouvellement : le `--deploy-hook` de la ligne cron est redondant.
- Certificats : `api.relaisdesk.fr` (2027-01-31), `informatiqueadomicile03.fr` (2027-01-15, couvre l'API historique), `informatiqueadomicile03.fr-0001` (2027-01-20, utilisé par le vhost PHP), `relaisdesk.duckdns.org` (2027-01-12, authentificateur `manual`, aucune référence dans nginx, /opt/relaisdesk, systemd, wireguard : non renouvelable automatiquement, donc expirera).
- Les trois certificats nginx ont `authenticator = installer = nginx` ; le snap embarque son plugin nginx. Le paquet `python3-certbot-nginx` n'est utile qu'au certbot apt.

### sshd
- `permitrootlogin without-password`, `x11forwarding yes`, `passwordauthentication no`. Un seul drop-in existe (`60-cloudimg-settings.conf`).
- La clé root a déjà `command="echo 'Please login as the user ubuntu...';sleep 10;exit 142"` + `no-port-forwarding,no-agent-forwarding,no-X11-forwarding` : root ne peut déjà pas obtenir de shell. `PermitRootLogin no` est donc une défense en profondeur sans risque d'accès (l'accès se fait par `ubuntu` + `sudo -n`).

### Mises à jour
22 paquets (`apt-get -s dist-upgrade` donne le même résultat que `upgrade`, 0 retenu, aucun hold) : `docker-ce`, `docker-ce-cli`, `containerd.io`, plugins docker, `snapd`, `libglib2.0-*`, `open-iscsi`, `open-vm-tools`, `ubuntu-pro-client`, `tmux`, `nano`, `xfsprogs`, `distro-info`. Le noyau est un méta-paquet `linux-image-oracle` déjà à 6.8.0-1067 : pas de `dist-upgrade` nécessaire. Anciens noyaux : 1062 encore installé (nettoyage `autoremove` laissé à l'exploitant).

### Autres
- WireGuard : pair `ZVQAACvA5TpkT7q5Bby1ybUOGPHQL+k7hWmjaayZZCc=` sans handshake depuis environ 37 jours. Information seulement, aucune action prévue.
- Conf nginx globale : `ssl_protocols TLSv1 TLSv1.1 TLSv1.2 TLSv1.3;` et `# server_tokens off;`.

## 2. Ce que fait chaque sous-commande

| Sous-commande | Effet | Risque | Coupure |
|---|---|---|---|
| `backup` | Copie `/etc/nginx`, `/etc/letsencrypt`, `/etc/ssh`, `/etc/iptables`, `iptables-save`, crontab root, liste dpkg dans `/root/hardening-20261117-<ts>/` (700) | Aucun | Aucune |
| `nginx` | `ssl_protocols TLSv1.2 TLSv1.3` et `server_tokens off` globaux ; réécriture du vhost historique (TLS 1.2/1.3, `proxy_ssl_verify`, `X-Forwarded-For $remote_addr`, `CF-Connecting-IP ""`, 11 Mo, location downloads, journal dédié) ; `nginx -t` puis reload ; test santé des deux domaines ; **restauration automatique** si `-t` ou la santé échoue | Un client TLS 1.0/1.1 historique serait refusé (probabilité très faible) ; vérification amont activée sur le vhost historique (déjà validée sur l'autre) | Aucune (reload) |
| `sshd` | Drop-in `90-relaisdesk-hardening.conf` : `X11Forwarding no`, `PermitRootLogin no` ; garde-fous (utilisateur `ubuntu` + clé présents) ; `sshd -t` puis reload | Erreur de manipulation = perte d'accès : **garder la session ouverte et tester une 2e connexion** | Aucune |
| `certbot` | Dry-run snap sur les 3 certificats nginx **d'abord** ; si OK : retire la ligne cron, désactive `certbot.timer`, retire le paquet apt `certbot` (simulation préalable, abandon si apt retirerait autre chose ; `python3-certbot*` conservés, pas d'autoremove ni de purge) | Le dry-run nginx recharge brièvement nginx ; si le snap échoue, rien n'est modifié | Aucune |
| `certbot-delete-duckdns` | Refuse si le nom est référencé ; confirmation en tapant `SUPPRIMER` ; `certbot delete` | Irréversible (copie dans la sauvegarde) | Aucune |
| `iptables` | Garde 1 règle 51820/udp en live et 1 seule dans `rules.v4` (validé par `iptables-restore --test`) | Aucun : la règle persistante couvre l'ordre de démarrage | Aucune |
| `upgrade` | `apt-get update`, liste, confirmation, `upgrade` (needrestart en mode liste seulement), indique si un reboot est requis | **Mise à jour de docker-ce : le démon redémarre, hbbs/hbbr coupent quelques secondes** (restart policy les relance) ; `containerd`/`snapd` idem | hbbs/hbbr ~10-30 s ; API non concernée |
| `verify` | Services actifs/enabled, conteneurs, auth hbbs/hbbr, ports 80/443/21115-21117/tcp, 21116/udp, 51820/udp, 8443 local, santé des 2 domaines, `Server` sans version, TLS, sshd, doublons iptables, certbot, reboot en attente, noyau | Lecture seule | Aucune |
| `reboot-and-check` | Affiche seulement la procédure ; ne redémarre jamais | Aucun | Voir §4 |

## 3. Ordre d'exécution recommandé
1. `backup`
2. `nginx` (puis `verify`)
3. `sshd` : **session ouverte conservée**, test d'une nouvelle connexion `ubuntu` + `sudo -n true`, sinon `rollback-sshd`
4. `certbot` (après `nginx`, pour que le dry-run teste la configuration finale)
5. `iptables`
6. `upgrade` en heure creuse (coupure hbbs/hbbr de quelques secondes), puis `verify`
7. Optionnel : `certbot-delete-duckdns` ; plus tard, selon `api-legacy.access.log`, suppression du vhost historique ; `reboot-and-check` seulement si `/var/run/reboot-required` apparaît après l'upgrade.

Déploiement : `scp -i .secrets/oracle_openssh scripts/hardening-oracle-20261117.sh ubuntu@79.72.27.213:/tmp/` puis `sudo bash /tmp/hardening-oracle-20261117.sh <étape>`.

## 4. Coupure attendue
- Étapes 1 à 5 : aucune coupure.
- `upgrade` : hbbs/hbbr indisponibles brièvement (redémarrage de Docker). Les sessions RustDesk en cours peuvent se couper et se rétablissent seules.
- Reboot complet (non nécessaire a priori après celui du jour) : API, hbbs/hbbr et le site PHP indisponibles environ 1 à 2 minutes ; le reboot du jour a confirmé que tout revient seul.

## 5. Retour arrière
- `rollback-nginx` : restaure `nginx.conf` et le vhost historique depuis la plus ancienne sauvegarde (état d'origine), `nginx -t`, reload. Automatique en cas d'échec pendant l'étape.
- `rollback-sshd` : supprime le drop-in, `sshd -t`, reload.
- `rollback-iptables` : restaure `rules.v4` d'origine (le live n'est pas modifié, une règle suffit).
- `rollback-certbot-cron` : restaure le crontab root, réinstalle `certbot` apt si besoin, réactive `certbot.timer`. Un certificat supprimé se récupère depuis `etc-letsencrypt` de la sauvegarde.
- `upgrade` : pas de rollback scripté ; les paquets précédents sont dans les caches apt si nécessaire. Sauvegarde = `/root/hardening-20261117-<ts>/`. `BACKUP_DIR=...` force une sauvegarde précise.
- En dernier recours si SSH est perdu : console série Oracle Cloud.

## 6. Points restant à l'appréciation de l'exploitant
- Supprimer ou non le vhost historique (après mesure via `api-legacy.access.log`), et le CORS correspondant côté API (code non modifié ici).
- Doublons `MASQUERADE -o ens3` (même cause) : à traiter séparément en alignant `wg0.conf` et `rules.v4`.
- Pair WireGuard inactif depuis environ 37 jours.
- Anciens noyaux (`autoremove`) et paquet `python3-certbot-nginx` désormais inutile.
- Constat de lecture : un test `POST /api/heartbeat` (404) provenant de `79.72.27.213` apparaît dans le journal de l'API (supervision locale probable).
