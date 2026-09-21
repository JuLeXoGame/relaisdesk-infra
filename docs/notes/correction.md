# Décision d'architecture — RustDesk Community sans proxy

Ce document remplace intégralement l'ancienne proposition SOCKS5.

## Topologie retenue

- API de licences RelaisDesk : `https://api.relaisdesk.fr`
- RustDesk `hbbs` : `21115/tcp`, `21116/tcp` et `21116/udp`
- RustDesk `hbbr` : `21117/tcp`
- Aucun proxy SOCKS5 et aucun port `1080/tcp`
- Aucun RustDesk Server Pro

## RustDesk2.toml

```toml
rendezvous_server = 'api.relaisdesk.fr:21116'
nat_type = 1
serial = 0

[options]
custom-rendezvous-server = 'api.relaisdesk.fr'
relay-server = 'api.relaisdesk.fr'
key = '{public_key}'
```

`{public_key}` est le contenu de `id_ed25519.pub`, sans accolades dans le
fichier réel. Le champ `api-server` n'est pas défini : l'API RelaisDesk est une
API de licences personnalisée, pas l'API de RustDesk Server Pro. L'option
`disable-udp` n'est pas définie afin de conserver la perforation NAT sur
`21116/udp`.

Côté serveur, vérifier aussi la politique de clé de `hbbr`. Dans le serveur OSS,
sa valeur par défaut est vide ; utiliser la même clé non vide que `hbbs` si vous
voulez empêcher l'utilisation du relais sans clé correspondante.

## Limite de sécurité connue

Un code viewer expiré ne permet plus une nouvelle activation via l'API
RelaisDesk. En revanche, RustDesk Community ne consulte pas cette API à chaque
connexion. Un client qui a déjà conservé la configuration et la clé publique
peut donc encore joindre `hbbs`/`hbbr` après l'expiration du code.

Ne pas présenter les codes de 12 heures comme une révocation réseau. Une telle
garantie demanderait un contrôle d'accès dans le chemin réseau ou dans le
serveur RustDesk, ou une fonctionnalité équivalente de RustDesk Server Pro.
