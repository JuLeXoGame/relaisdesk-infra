# Envois `/api/heartbeat` et `/api/sysinfo` inutiles — 18 novembre 2026

Version produit inchangée : **1.0.0**. L'API RelaisDesk n'est pas redéployée.

## Constat

Dans les journaux nginx du 17 et du 18 novembre, `api.relaisdesk.fr` a reçu
5 684 `POST /api/heartbeat` et 776 `POST /api/sysinfo`, tous en **404**. Il les
reçoit toutes les 15 s et toutes les 2 min, sans User-Agent, depuis
`79.72.27.213`. Une capture de 40 s sur le serveur montre que ces connexions
entrent par `ens3`. Elles viennent du pair WireGuard actif `10.66.66.2`, un
poste RelaisDesk, et ressortent avec l'IP publique du serveur à cause de la
règle `MASQUERADE`. Ce n'est pas une supervision locale, contrairement à ce
qu'indiquait le rapport de durcissement (corrigé). Le cron
`betterstack-heartbeat.sh` est sans rapport : il appelle Better Stack, à
l'extérieur.

## Ce que fait ce heartbeat (vérifié dans le code)

Deux heartbeats différents partent des postes :

| Envoi | Émetteur | Rôle | Réponse |
| --- | --- | --- | --- |
| `POST /api/v1/devices/heartbeat` | agent de flotte Go (`installer/viewer/fleet.go:119`) | poste en ligne, jeton d'autorisation réseau, réveil (Wake-on-LAN), mises à jour (`api/handlers/devices.go:184`) | 200, **indispensable** |
| `POST /api/heartbeat` puis `/api/sysinfo` | moteur RustDesk (`rustdesk/src/hbbs_http/sync.rs`) | protocole du serveur commercial RustDesk Pro | 404, **inutile** |

Pour le second :

- L'API RelaisDesk n'a aucune route `/api/heartbeat`, `/api/sysinfo` ni
  aucune autre route RustDesk Pro. `git log -S` ne trouve aucune trace d'une
  route `/api/heartbeat` dans `api/`, `docs/`, `scripts/` ou `installer/`.
- Le moteur lance cette synchronisation (`rendezvous_mediator.rs:123`) dès que
  `api-server` est renseigné. Les installateurs le renseignent sur
  `https://api.relaisdesk.fr`, et doivent continuer à le faire : sinon le
  moteur se rabattrait sur `http://<serveur hbbs>:21114`.
- La réponse au heartbeat n'est lue que si elle est du JSON. Elle peut alors
  porter `disconnect` (couper des sessions), `strategy` (réécrire des options
  du client) ou `sysinfo`. Avec un 404, rien n'est lu : aucun de ces
  mécanismes n'a jamais servi.
- `/api/sysinfo` envoie le nom d'hôte, le nom d'utilisateur, le système et le
  matériel. Faute de réponse `SYSINFO_UPDATED`, l'envoi recommence toutes les
  2 minutes. Les données ne sont pas conservées : nginx ne journalise pas le
  corps des requêtes et l'API rejette la route.
- `is_pro()` reste faux, puisque seul `SYSINFO_UPDATED` le passerait à vrai. Il
  conditionne l'option `hide_cm` (`ipc.rs:939`, déjà couverte par
  `is_custom_client()`) et un message d'erreur Windows (`connection.rs:2797`).
  Il ne faut donc **jamais** répondre `SYSINFO_UPDATED` à ces routes.
- Pendant une session active, le heartbeat part toutes les 3 s au lieu de 15 s.
- En cas de 5xx, le client retente par le relais TCP (`common.rs:1351`), ce qui
  ajoutait une charge inutile lors des 502 intermittents.

Conclusion : ces envois ne servent à rien pour RelaisDesk. La présence des
postes persistants repose entièrement sur `/api/v1/devices/heartbeat`.

## Corrections

### Serveur (appliqué le 18 novembre à 02:56 UTC)

Vhost `/etc/nginx/sites-available/relaisdesk-api` :

```nginx
location = /api/heartbeat { return 204; }
location = /api/sysinfo { return 204; }
```

Le client traite une réponse vide exactement comme le 404 : la réponse au
heartbeat n'est pas lue, `sysinfo` est renvoyé deux minutes plus tard et
`is_pro()` reste faux. Ces requêtes ne passent plus par l'API, ne sont plus
exposées aux 502 et n'apparaissent plus en 404 dans les journaux.

- Avant le rechargement, le fichier était le même que celui mesuré à
  l'installation (SHA-256 `5b6419a0…1243`). `nginx -t` OK, rechargement OK.
- Sauvegarde : `/root/hardening-20261117-20261118-012152/relaisdesk-api.avant-legacy-204`.
- Contrôles externes : les deux routes répondent 204 (0 octet) ;
  `/api/v1/devices/heartbeat` vide répond 400 « Identifiant manquant », preuve
  qu'il atteint toujours l'API ; santé 200 ; manifeste des téléchargements
  200 ; un chemin voisin (`/api/heartbeatX`) répond toujours 404 ; ancien
  domaine 200.
- Le vrai poste : voir « Observation » ci-dessous.
- Modèle versionné mis à jour : `scripts/nginx-api-relaisdesk.conf.example`.
  Ce modèle était déjà en retard sur la production (bloc des téléchargements
  et archive des sources absents) ; il n'est pas resynchronisé ici.

Retour arrière : remettre la sauvegarde, puis `nginx -t` et
`systemctl reload nginx`.

### Moteur client (préparé, non distribué)

Fork `rustdesk`, branche locale `codex/no-pro-sync-20261118` (base
`relaisdesk/relaisdesk/authorization`, a5043e4ed), commit `3e5e68387` :
`heartbeat_url()` renvoie une URL vide, donc la boucle de synchronisation ne
fait plus rien. C'était déjà son comportement pour un serveur public, et
`is_pro()` reste faux. Un test unitaire vérifie que l'URL est vide.

- `cargo +1.99.1 check --locked --lib --tests` réussit sans nouvel
  avertissement. Le seul avertissement dans `sync.rs` est la dépréciation
  `base64::encode` du code d'origine.
- Le test unitaire n'a pas été exécuté : il faudrait compiler entièrement
  toutes les dépendances, ce que le disque C: (98 % plein) ne permet pas
  sereinement.
- Effet seulement à la prochaine reconstruction des clients. Les postes
  installés continueront d'envoyer ces requêtes, auxquelles nginx répond
  désormais 204.
- Non poussé. Indépendant de la branche `codex/deps-20261117`.

## Effet sur l'essai `proxy_ssl_session_reuse off`

Les anciennes routes représentaient la plupart des 502 comptés (54 sur 75).
Depuis 02:56 UTC, elles ne passent plus par l'API, donc le nombre brut de 502
baisse quoi qu'il arrive. Le relevé programmé du 19 novembre a été modifié
pour mesurer le taux **hors** `/api/heartbeat` et `/api/sysinfo`, avant comme
après.

## Observation du vrai poste (02:56 à 03:00 UTC)

Journal nginx, requêtes de `79.72.27.213` :

- `/api/heartbeat` : 204, toutes les 15 à 18 s (même cadence qu'avant).
- `/api/sysinfo` : 204, une fois sur la fenêtre (cadence de 2 min inchangée).
- `/api/v1/devices/heartbeat` : 200, toutes les 40 à 50 s. Le poste reste en
  ligne et continue de recevoir son jeton réseau.

Journal de l'API : 26 appels `/api/heartbeat` ou `/api/sysinfo` entre 02:50 et
02:56:11, aucun ensuite. Les heartbeats `/api/v1/devices/heartbeat` continuent
d'arriver. `NRestarts=0`. Aucun 502 dans nginx depuis 02:56:12 (fenêtre courte,
non significative pour l'essai sur les 502).

Le comportement du poste n'a pas changé. Seule la réponse aux deux anciennes
routes est différente, et ces routes ne passent plus par l'API.
