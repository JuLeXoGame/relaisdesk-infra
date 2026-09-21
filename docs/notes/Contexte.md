# Contexte actuel de RelaisDesk

Ce document décrit uniquement l’architecture active. L’ancien modèle SOCKS5 est
historique et ne doit plus être construit, configuré ni déployé.

## Architecture

RelaisDesk utilise RustDesk Community avec les services libres `hbbs` et `hbbr`.
Les applications Technicien et Viewer se connectent directement à :

- `api.relaisdesk.fr:21116` pour le rendez-vous `hbbs` ;
- `api.relaisdesk.fr:21117` pour le relais `hbbr` ;
- `api.relaisdesk.fr` pour l’API RelaisDesk en HTTPS.

Il n’existe aucun proxy SOCKS5, aucun port `1080`, aucun identifiant SOCKS dans
RustDesk et aucun `api-server` RustDesk Server Pro.

## Configuration RustDesk attendue

```toml
rendezvous_server = 'api.relaisdesk.fr:21116'
nat_type = 1
serial = 0

[options]
custom-rendezvous-server = 'api.relaisdesk.fr'
relay-server = 'api.relaisdesk.fr'
key = '{public_key}'
```

`api-server` ne doit pas être ajouté : il appartient aux fonctions du serveur
RustDesk Pro et n’est pas nécessaire au fonctionnement Community direct.

## Composants du projet

- `api/` : authentification, commandes, paiements, factures et API des codes Viewer.
- `database/` : base SQLite, licences, sessions, commandes et factures.
- `installer/configurator/` : application Technicien ; valide la licence et écrit la configuration RustDesk.
- `installer/viewer/` : application client ; valide un code temporaire et écrit la même configuration serveur.
- `dashboard_admin/` : application d’administration native.
- `keygen/` : outil local de gestion des licences.
- `relaisdesk/` : site public et espaces web privés.
- `rustdesk/` : source amont RustDesk Community 1.4.9.
- `scripts/` : installation, pare-feu et déploiement serveur.

## Flux Technicien

1. Le Technicien saisit son identifiant et sa clé de licence.
2. L’application appelle `/api/v1/activate` en HTTPS.
3. L’API contrôle la licence et renvoie la clé publique RustDesk et les ports.
4. L’application valide ces valeurs puis écrit `RustDesk2.toml`.
5. RustDesk contacte directement `hbbs`/`hbbr`.
6. Le tableau de bord Technicien utilise un jeton de session API pour créer et révoquer des codes Viewer.

## Flux Viewer

1. Le Technicien génère un code temporaire valable douze heures.
2. Le client saisit ce code dans le Viewer.
3. Le Viewer appelle `/api/v1/viewer/activate` en HTTPS.
4. L’application valide la réponse, configure RustDesk puis annonce l’ID RustDesk obtenu.
5. RustDesk contacte directement `hbbs`/`hbbr` avec la clé publique du serveur.

## Limite importante de RustDesk Community

Une fois `RustDesk2.toml` écrit, l’expiration d’une licence ou d’un code bloque
les nouvelles opérations dans les applications RelaisDesk, mais ne constitue
pas à elle seule une révocation réseau de la configuration RustDesk déjà copiée.
Cette limite doit être prise en compte dans les CGV, la communication commerciale
et le modèle de sécurité. La clé publique n’est pas un secret d’autorisation.

## Réseau serveur

Les règles actives sont décrites dans `scripts/firewall.sh` et
`docs/DEPLOYMENT.md`. L’ancien port `1080/tcp` doit rester fermé. Les ports
RustDesk Community nécessaires sont exposés conformément au déploiement
`hbbs`/`hbbr`, et l’API est publiée exclusivement en HTTPS en production.

## Secrets

Les secrets réels doivent résider hors du dépôt, avec des permissions minimales :

- `ADMIN_TOKEN`, secrets Stripe et SMTP dans `/etc/relaisdesk/api.env` ;
- base SQLite et factures dans `/data/relaisdesk` ;
- clé privée `hbbs` uniquement sur le serveur ;
- aucune clé de licence ou jeton DNS dans les sources et artefacts publics.
