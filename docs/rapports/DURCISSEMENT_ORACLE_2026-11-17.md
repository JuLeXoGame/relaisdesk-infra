# Durcissement du serveur Oracle - plan du 2026-11-17

Statut : **partiellement appliqué le 18 novembre 2026** (voir §0) ; `nginx` et `certbot-delete-duckdns` restent à faire. Tous les relevés ci-dessous ont été faits en lecture seule sur `79.72.27.213`. Le script `scripts/hardening-oracle-20261117.sh` doit être copié sur le serveur et lancé en root, une sous-commande à la fois, après validation de l'exploitant.

## 0. Application du 18 novembre 2026 (01:21-01:45 UTC)

Autorisée par l'exploitant : `backup`, `sshd`, `certbot`, `iptables`, `upgrade`, `verify`. Sauvegarde : `/root/hardening-20261117-20261118-012152/`.

| Étape | Résultat |
|---|---|
| `backup` | OK (692 Ko, root seul) |
| `sshd` | Drop-in appliqué : `PermitRootLogin no`, `X11Forwarding no` ; une 2ᵉ connexion `ubuntu` + `sudo -n` a été testée avec succès |
| `certbot` | Cron root retiré, `certbot.timer` apt désactivé, `snap.certbot.renew.timer` actif. Le paquet apt n'a **pas** été retiré : l'abandon de sécurité du script s'est déclenché (apt aurait aussi retiré `python3-certbot-nginx`) ; il reste installé, inactif. Dry-run snap réussi pour `api.relaisdesk.fr`, `informatiqueadomicile03.fr-0001` et `informatiqueadomicile03.fr`. Le certificat `relaisdesk.duckdns.org` (authentificateur manuel) est ignoré. |
| `iptables` | 4 règles 51820/udp en live ramenées à 1 ; `rules.v4` ramené de 3 à 1 ; REJECT final et autres ports inchangés |
| `upgrade` | 01:44 UTC : 24 paquets mis à jour, aucun retiré (docker-ce 29.8.2 vers 29.10.0, containerd 2.3.6 vers 2.4.2, snapd, glib, etc.). hbbs/hbbr redémarrés par Docker (restart policy), joignables ensuite ; API non redémarrée (`NRestarts=0`) ; pas de redémarrage du serveur requis ; `verify` 43 OK, 0 échec |
| `verify` | 43 OK, 2 avertissements (nginx, hors étape), 0 échec ; santé des deux domaines 200 ; ports 21115-21117 et 443 joignables depuis l'extérieur |

Deux défauts du script ont été trouvés et corrigés pendant l'application : `certbot renew` ne retient qu'un seul `--cert-name` (le dry-run n'avait testé qu'un certificat ; un dry-run par certificat est maintenant fait) et le contrôle « en écoute » de `verify` lisait la mauvaise colonne de `ss`.

Non fait : `nginx` (l'autre session éditait nginx), suppression du certificat duckdns, doublons `MASQUERADE`.

### Anomalie préexistante constatée : 502 intermittents sur `api.relaisdesk.fr`

**Constat (lecture seule, 18 novembre)** : environ 2,5 % des requêtes reçoivent un 502, chaque jour depuis au moins le 4 novembre (50 à 106 par jour pour ~2 700 réponses 200 ; le journal le plus ancien conservé date du 4). Ce n'est donc pas lié au durcissement ni à l'upgrade. `error.log` : « upstream SSL certificate verify error: (20:unable to get local issuer certificate) » vers `https://127.0.0.1:8443`. La plupart des requêtes touchées sont les `POST /api/heartbeat`, `/api/sysinfo` et `/api/v1/devices/heartbeat` émis par l'IP du serveur lui-même (0,8 % d'entre elles), et quelques `GET /api/v1/health` externes (~1 %). Le certificat de l'API a été renouvelé le 2 novembre (chaîne Let's Encrypt « YE1 / Root YE », 4 certificats avec X2 recroisé par X1).

**Éliminé** : l'API sert toujours la même chaîne (400 poignées de main : 4 certificats, validation OK à profondeur 3) ; un nginx temporaire avec la même configuration, avec ou sans réutilisation de session TLS amont, profondeur 3, 5 ou 9, n'a eu aucun échec sur 750 requêtes ; 120 requêtes séquentielles et 400 parallèles (25 en même temps) sur le nginx de production n'en ont eu aucun non plus ; les échecs ne viennent pas d'un worker précis ; le vhost historique, sans vérification amont, ne les subit pas.

**Cause non établie.** Pistes restantes : (1) capturer une poignée de main en échec (`tcpdump` est installé ; capture en boucle sur `lo` port 8443, puis rapprochement avec l'heure d'une erreur) ; (2) essayer `proxy_ssl_session_reuse off` sur le vhost de production et mesurer le taux pendant 24 h ; (3) palliatif : `proxy_ssl_verify off` sur cette liaison loopback (le trafic ne quitte pas la machine). Toute modification du vhost de production est à décider par l'exploitant.

**Essai en cours (autorisé par l'exploitant) : `proxy_ssl_session_reuse off`.** Appliqué le 18 novembre à 01:58 UTC dans les deux blocs `proxy_pass` de `/etc/nginx/sites-available/relaisdesk-api` (`nginx -t` OK, rechargement OK, santé 200). Sauvegarde de l'état précédent : `/root/hardening-20261117-20261118-012152/relaisdesk-api.avant-session-reuse` (SHA-256 d'origine `bcda9293…ce22`). Base de mesure : 11 réponses 502 dans l'`access.log` courant au moment du changement (journal du 18 novembre démarré à 00:00 UTC) ; taux antérieur ~50 à 100 par jour. À relire après 24 h : `sudo awk '$4 >= "[18/Nov/2026:02" && $9==502' /var/log/nginx/access.log | wc -l` ; si le taux reste identique, remettre l'ancien fichier (`sudo cp -a <sauvegarde> /etc/nginx/sites-available/relaisdesk-api && sudo nginx -t && sudo systemctl reload nginx`) et passer à la capture `tcpdump`.

## 1. Constat (relevés du 2026-11-17)

### Événement notable pendant l'audit
Le serveur a **redémarré à 17:05 UTC** (hors de cet audit) : noyau passé de 6.8.0-1062 à **6.8.0-1067-oracle**. Au retour, tous les services (API, nginx, Docker, wg0, php8.1-fpm, MariaDB) et les deux conteneurs étaient actifs, les deux domaines répondaient 200. Le redémarrage « noyau + libc » n'est donc plus à faire ; ce reboot a servi de test de reprise.

### Domaine historique `api.informatiqueadomicile03.fr`
- Résolu en DNS vers la même IP ; vhost proxifiant tout vers l'API (aucune des protections du vhost `relaisdesk-api`).
- Le certificat utilisé (`live/informatiqueadomicile03.fr`) couvre bien `api.informatiqueadomicile03.fr` ; expire le 2027-01-15.
- **Usage réel non mesurable aujourd'hui** : le journal nginx global n'enregistre pas le nom d'hôte, le vhost n'a pas de journal propre et l'API ne journalise ni Host ni Origin. Ordre de grandeur du trafic total (tous vhosts) : environ 42 000 lignes/jour, dont 1 918 `POST /api/v1/devices/heartbeat` et 511 `/api/v1/health` sur la veille ; une partie vient de `79.72.27.213` : c'est le trafic d'un poste relié par WireGuard (pair `10.66.66.2`), qui ressort avec l'IP publique du serveur (correction du 18 novembre). Rien ne permet d'attribuer ce trafic à l'ancien domaine.
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
- Pair WireGuard `10.66.66.3` inactif depuis environ 37 jours (l'autre pair, `10.66.66.2`, est actif).
- Anciens noyaux (`autoremove`) et paquet `python3-certbot-nginx` désormais inutile.
- ~~Constat de lecture : un test `POST /api/heartbeat` (404) provenant de `79.72.27.213` (supervision locale probable).~~ Corrigé le 18 novembre : ce sont les envois de l'ancien protocole RustDesk Pro par le moteur client d'un poste relié par WireGuard ; traité, voir [HEARTBEAT_RUSTDESK_PRO_2026-11-18.md](HEARTBEAT_RUSTDESK_PRO_2026-11-18.md).
