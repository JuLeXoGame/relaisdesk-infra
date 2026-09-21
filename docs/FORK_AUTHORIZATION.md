# Autorisation réseau des forks RelaisDesk

## Pourquoi deux paires de clés

La paire RustDesk `id_ed25519` / `id_ed25519.pub` identifie `hbbs` et `hbbr` aux
clients. La faire tourner invalide la valeur `key` de tous les clients et crée
une coupure générale. Elle ne constitue pas une licence et doit rester stable,
sauf compromission réelle.

RelaisDesk utilise donc une seconde paire Ed25519 :

- la clé privée reste dans l'API et signe des jetons `rd1` ;
- `hbbs` et `hbbr` ne reçoivent que la ou les clés publiques ;
- les jetons durent par défaut 300 secondes ;
- le `kid` permet une rotation progressive sans interrompre les clients.

La table `server_keys` continue de stocker la clé publique RustDesk distribuée
dans `RustDesk2.toml`. Elle ne doit pas recevoir la clé d'autorisation.

## Format et propriétés

Un jeton compact a la forme :

```text
rd1.<payload-json-base64url>.<signature-ed25519-base64url>
```

Le payload contient l'émetteur, l'audience, le sujet, le locataire, le rôle,
un identifiant aléatoire, la clé publique de preuve de l'appareil, le `kid`, les
dates et `max_sessions`. Aucune clé de licence ni aucun code viewer n'y figure.

Chaque requête sensible est aussi signée par la clé privée locale sur une forme
canonique :

```text
relaisdesk-proof-v1\n<action>\n<timestamp>\n<nonce>\n<token>
```

Le serveur accepte une dérive d'horloge de 30 secondes et mémorise chaque nonce
jusqu'à l'expiration du jeton. La preuve est liée à l'action et ne peut donc pas
être réutilisée sur une autre cible ou un autre relais.

## Création initiale

Sur une machine d'administration sécurisée :

```bash
cd api
go run ./cmd/network-keygen -out /tmp/network-auth-ed25519
sudo install -m 0600 -o relaisdesk -g relaisdesk \
  /tmp/network-auth-ed25519 /etc/relaisdesk/network-auth-ed25519
rm /tmp/network-auth-ed25519
```

La commande affiche uniquement la clé publique. Configurer l'API :

```text
NETWORK_AUTH_PRIVATE_KEY_FILE=/etc/relaisdesk/network-auth-ed25519
NETWORK_AUTH_KEY_ID=relaisdesk-1
NETWORK_TOKEN_TTL_SECONDS=300
```

Configurer **à l'identique** `hbbs` et `hbbr` :

```text
RELAISDESK_AUTH_REQUIRED=Y
RELAISDESK_AUTH_PUBLIC_KEYS=relaisdesk-1=<cle-publique-base64url>
```

Ne jamais placer la clé privée dans Docker Compose, dans Git, dans une image ou
sur les serveurs RustDesk.

## Rotation sans coupure

1. Générer une nouvelle paire avec `kid=relaisdesk-2`.
2. Ajouter sa clé publique aux deux serveurs **avant** de changer l'API :

   ```text
   RELAISDESK_AUTH_PUBLIC_KEYS=relaisdesk-1=<ancienne>,relaisdesk-2=<nouvelle>
   ```

3. Redémarrer progressivement `hbbs` et `hbbr`, puis vérifier leur démarrage.
4. Configurer l'API avec la nouvelle clé privée et `NETWORK_AUTH_KEY_ID=relaisdesk-2`.
5. Attendre au moins la durée maximale d'un jeton plus la dérive d'horloge
   (actuellement 930 secondes).
6. Retirer `relaisdesk-1` des deux serveurs.

Une rotation mensuelle est raisonnable pour cette clé d'autorisation, mais ne
remplace ni la révocation des licences ni la protection du fichier privé.

## Configuration cliente générée

Le configurateur produit une configuration sans proxy :

```toml
rendezvous_server = 'api.relaisdesk.fr:21116'
nat_type = 1
serial = 0

[options]
custom-rendezvous-server = 'api.relaisdesk.fr'
relay-server = 'api.relaisdesk.fr'
api-server = 'https://api.relaisdesk.fr'
key = '<cle-publique-id_ed25519.pub>'
relaisdesk-token-file = '<chemin-prive>/network-token'
relaisdesk-proof-key-file = '<chemin-prive>/proof-key'
```

`serial = 0` est optionnel et sans incidence sur l'absence de proxy. La valeur
`api-server` est une URL HTTPS normale ; les serveurs rendez-vous et relais ne
doivent pas contenir de schéma. Les deux options `relaisdesk-*` appartiennent au
fork et sont inconnues d'un binaire RustDesk amont.

## Ordre de mise en production

1. Construire et publier le code source correspondant des forks AGPL.
2. Construire le client forké Windows et Linux, puis appeler
   `installer/build.ps1` avec les trois artefacts et la paire de signature de
   version décrite dans `RELEASE_SIGNING.md`. Le script calcule et injecte les
   empreintes SHA-256 ; il refuse de construire sans eux.
3. Déployer l'API avec sa clé privée.
4. Déployer `hbbs`/`hbbr` avec les clés publiques, d'abord dans un environnement
   de recette.
5. Tester inscription viewer, connexion technicien, P2P, relais, révocation,
   quota et renouvellement pendant plus de cinq minutes.
6. Activer `RELAISDESK_AUTH_REQUIRED=Y` en production seulement quand tous les
   binaires distribués comprennent le protocole.

Un binaire RustDesk officiel non modifié ne possède ni les champs protobuf ni
la preuve locale et sera volontairement refusé en mode obligatoire.

## AGPL et légalité

Les forks conservent l'AGPL v3. Pour chaque binaire distribué et service modifié
mis à disposition sur un réseau, fournir un accès clair au code source
correspondant, aux sous-modules modifiés, aux scripts de build et aux avis de
licence. RelaisDesk peut facturer le service, l'hébergement, le support et ses
fonctionnalités ; il ne peut pas retirer les libertés accordées par l'AGPL sur
le code dérivé. Faire relire les CGV, la politique de confidentialité et la
procédure de publication des sources par un juriste avant la production.
