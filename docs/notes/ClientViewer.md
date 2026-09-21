# Application Client Viewer

Le Viewer est destiné au client final assisté par un technicien. Il utilise
RustDesk Community en connexion directe vers `hbbs`/`hbbr` et ne contient
aucun proxy SOCKS5.

## Parcours

1. Le technicien génère un code `XXXX-XXXX` depuis son espace authentifié.
2. Le code est valable douze heures et rattaché à la licence du technicien.
3. Le client saisit le code dans le Viewer.
4. Le Viewer appelle `POST /api/v1/viewer/activate` en HTTPS.
5. L’API vérifie le code et la licence parente, puis renvoie la clé publique et l’expiration.
6. Le Viewer valide strictement la réponse, écrit `RustDesk2.toml`, lance RustDesk et annonce son ID avec `POST /api/v1/viewer/announce`.

## Configuration écrite

```toml
rendezvous_server = 'api.relaisdesk.fr:21116'
nat_type = 1
serial = 0

[options]
custom-rendezvous-server = 'api.relaisdesk.fr'
relay-server = 'api.relaisdesk.fr'
key = '{public_key}'
```

Les champs `api-server`, `proxy-*`, `[socks]`, `disable-udp` et le port `1080`
sont interdits dans cette architecture Community.

## Contrôles de sécurité

- HTTPS vérifié, délai réseau de dix secondes et limite de débit côté API.
- Format du code, ID RustDesk, nom d’hôte, port, clé publique et expiration validés.
- Configuration écrite avec permissions privées.
- Un code ne peut annoncer qu’un seul ID RustDesk.
- La licence parente est revérifiée pendant l’activation.
- Le raccourci relance le Viewer RelaisDesk, pas RustDesk directement.
- Après le lancement et l’annonce de l’ID, `RustDesk2.toml` est supprimé ; une
  nouvelle exécution doit donc repasser par un code encore valide.

## Limite connue

La suppression locale limite la réutilisation sur un poste sain, mais elle ne
peut pas interrompre une instance RustDesk déjà lancée ni empêcher un utilisateur
administrateur de copier la configuration. RustDesk Community direct ne fournit
pas de révocation réseau centralisée.

## Construction

- Windows : le binaire RustDesk 1.4.9 officiel est intégré au Viewer.
- Linux : le paquet RustDesk 1.4.9 officiel est téléchargé depuis une URL figée,
  limité à 250 Mio et vérifié par SHA-256 avant installation.
- Les artefacts finaux sont produits par `installer/build.ps1`.
