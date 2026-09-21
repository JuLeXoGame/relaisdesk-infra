# Programmes RelaisDesk

## Composants actifs

| Composant | Source | Artefact principal |
|---|---|---|
| API de licences | `api/` | `api` (Linux serveur) |
| Base et logique métier | `database/` | bibliothèque Go |
| Technicien | `installer/configurator/` | `RelaisDesk_Technicien_Portable.exe` / `.deb` |
| Viewer | `installer/viewer/` | `RelaisDesk_Portable.exe` / `.deb` |
| Dashboard administrateur | `dashboard_admin/` | `dashboard_admin.exe` |
| Gestion locale des licences | `keygen/` | `keygen` |
| Site web | `relaisdesk/` | fichiers statiques |
| RustDesk Community | `rustdesk/` | source amont 1.4.9 |

Le dossier `archives/obsolete-proxy-artifacts/` est une archive technique et ne doit
jamais être déployé. Aucun service proxy n’est actif.

## Réseau

| Port | Protocole | Usage |
|---|---|---|
| 80/443 | TCP | site et certificats |
| 8443 ou 443 | TCP | API RelaisDesk HTTPS selon le frontal |
| 21115 | TCP | test de débit / NAT RustDesk |
| 21116 | TCP + UDP | rendez-vous `hbbs` |
| 21117 | TCP | relais `hbbr` |
| 21118-21119 | TCP | WebSocket RustDesk si activé |

Le port historique `1080/tcp` doit rester fermé.

## Construction

`installer/build.ps1` construit les applications Windows, les paquets Debian,
le serveur API Linux et les installateurs NSIS. Les artefacts publics sont
copiés dans `relaisdesk/downloads/`.

Chaque publication doit être reconstruite après modification des sources,
testée, signée lorsque possible et accompagnée de sommes SHA-256.
