# RelaisDesk RustDesk Licensing

Ecosysteme commercial autour de RustDesk Community. RelaisDesk maintient des
forks AGPL du client, de `hbbs` et de `hbbr` afin que les licences, rôles,
locataires et quotas soient contrôlés par le serveur réseau lui-même. Les
clients utilisent toujours les services libres `hbbs` et `hbbr` directement,
sans proxy SOCKS5 et sans RustDesk Server Pro.

## Où retrouver les fichiers

Rangement du 9 septembre 2026 : les sources et outils conservent leurs chemins.
Les fichiers anciens ont été déplacés, pas supprimés définitivement.

| Besoin | Emplacement |
| --- | --- |
| Guides et procédures actuels | [Index de la documentation](docs/README.md) |
| Dernières livraisons et correctif OVH | [Index des livraisons](output/README.md) |
| Résultats du script de maintenance | [Dossier update](update/README.md) |
| Anciennes livraisons et fichiers de test | [Archives](archives/README.md) |
| Notes, anciens audits et comparatif | `docs/notes/`, `docs/audits/`, `docs/rapports/` |
| Clés et identifiants locaux privés | `.secrets/` — ne jamais publier |

La clé Oracle est désormais dans `.secrets/oracle private.ppk` ; adapter son
chemin dans les éventuelles sessions PuTTY/WinSCP enregistrées. Les fichiers
`.env` restent à leur place pour ne pas modifier le fonctionnement du projet.
Les exemples sans secrets sont dans `docs/exemples/`.

Les dossiers `.cache/`, `.tools/`, `bin/` et les binaires embarqués sont conservés :
ils peuvent être nécessaires aux compilations. Aucun déploiement n'est effectué
par ce rangement. [Détail et journal des déplacements](docs/RANGEMENT_PROJET.md).

## Composants

Pour les prochaines modifications du site, travailler directement dans
`relaisdesk/` : **pas de nouvelle archive ni de copie de livraison web dans
`output/`, sauf demande explicite**. Les fichiers propres à Oracle restent
séparés. Les anciennes livraisons mentionnées ci-dessus sont conservées comme
historique.

- `database/` : schéma SQLite, locataires commerciaux stables, file persistante
  et sauvegardes cohérentes chiffrées.
- `keygen/` : CLI administrateur pour creer et gerer les licences.
- `api/` : API HTTP publique de validation et administration.
- `relaisdesk/client/` : espace client commercial sans mot de passe pour les
  licences, renouvellements, factures et historiques d'intervention.
- `installer/` : configurateur Windows/viewer et scripts NSIS.
- `rustdesk/` : fork du client avec preuve de possession et renouvellement à chaud.
- `rustdesk-server/` : forks `hbbs`/`hbbr` avec contrôle d'accès signé.
- `tests/` : tests d'integration.
- `docs/` : documentation d'exploitation.
- `scripts/` : scripts de deploiement serveur.

## Ports
 
- API licences : `443` via nom de domaine HTTPS (`api.relaisdesk.fr`).
- SSH : `22` / Web : `80`, `443`.
- RustDesk `hbbs` : `21115/tcp`, `21116/tcp` et `21116/udp`.
- RustDesk `hbbr` : `21117/tcp`.
- WebSocket RustDesk : `21118-21119/tcp` uniquement si un client Web est déployé.

## Build rapide

```bash
make build
make test
```

Sur Windows, l'installeur se construit avec :

```powershell
cd installer
.\build.ps1 `
  -RustDeskForkWindowsPath <exe-forke> `
  -RustDeskForkLinuxDebPath <deb-forke> `
  -RustDeskForkLinuxBinaryPath <elf-forke>
```

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [Deploiement](docs/DEPLOYMENT.md)
- [Administration](docs/ADMIN.md)
- [Guide client](docs/CLIENT_GUIDE.md)
- [Securite](docs/SECURITY.md)
- [Protocole et exploitation des forks](docs/FORK_AUTHORIZATION.md)
- [État d'implémentation des forks](docs/FORK_IMPLEMENTATION_STATUS.md)
- [Signature Windows gratuite avec SignPath Foundation](docs/SIGNPATH.md)
- [Signature et manifeste des versions](docs/RELEASE_SIGNING.md)
- [Migration OVH/Oracle depuis l'infrastructure existante](docs/MIGRATION_OVH_ORACLE.md)
- [Guide juridique avant publication, migration publique et ventes](docs/GUIDE_JURIDIQUE_MIGRATION_PRODUCTION.md)
- [Checklist juridique de mise en production](docs/LEGAL_RELEASE_CHECKLIST.md)
- [Audit de securite du 26 aout 2026](docs/audits/AUDIT_SECURITE_2026-08-26.md)
- [Audit SEO du 24 aout 2026](docs/SEO_RELAISDESK_2026-08-24.md)
