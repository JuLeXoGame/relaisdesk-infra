# Passage à Go 1.26.11 — 17 novembre 2026

La version produit RelaisDesk reste **1.0.0**. Rien n'est déployé par cette note.

## Motif

Un audit `govulncheck` en mode binaire de l'API déployée le 29 octobre
(`.cache/deploy-20261029/api`, SHA-256 `d1b4977d…`, identique à `bin/api` et à
l'exécutable actif sur Oracle) signale :

- [GO-2026-6859](https://pkg.go.dev/vuln/GO-2026-6859) : plantage du serveur
  HTTP/1 lié à une course sur les en-têtes avec `MaxBytesReader`, corrigé en
  Go 1.26.11. L'API utilise `http.MaxBytesReader` dans
  `api/middleware/logging.go` ; l'exécutable `relaisdesk-backup` est concerné
  par le même avis.
- [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932) : paquet OpenPGP
  abandonné de `golang.org/x/crypto` v0.56.0, non importé par les modules.

## Modifications

- Les huit modules Go passent de `go 1.26.10` à `go 1.26.11`.
- `golang.org/x/crypto` passe de v0.56.0 à la version publiée v0.59.0, avec
  `golang.org/x/sys` v0.50.0, dans les modules qui en dépendent (api, database,
  keygen, tests).
- Workflows `go-regressions.yml`, `build-macos.yml` et
  `package-macos-verified-engine.yml`, ainsi que `scripts/setup-wsl-go.sh` :
  Go 1.26.11. Aucune autre référence à 1.26.10 ne subsiste dans `scripts/` et
  `.github/workflows/`.

## Vérifications

- Linux (WSL, Go 1.26.11) : `go test ./...` réussi pour l'API (13 paquets, dont
  handlers, middleware, sauvegarde et paiements simulés), la base de données et
  les tests d'intégration.
- Windows : `go vet` et `go test` réussis pour keygen, dashboard_admin,
  installer/configurator, installer/viewer et installer/debpack (`-tags ci` ;
  les interfaces graphiques natives ne sont pas validées par ce mode).
- Exécutables Linux reconstruits (CGO désactivé, `-trimpath`, symboles réduits)
  dans `.cache/deploy-20261117/`, tous identifiés `go1.26.11` par `go version` :

| Exécutable | SHA-256 |
| --- | --- |
| api | `9b5583f1b687a1b357b7589c5d6f30be65225ac2eddd6b7b82fc72e7381b18ea` |
| relaisdesk-backup | `2fb68546d40dba8bd0e528f9b5f6cdd43e5838e67f3ad8b787c04edc2262ad5f` |
| relaisdesk-dbcheck | `8d6e5946b52fce7f93cac6429548fdf36072f80d5f93ff13fb15382b9be30f27` |
| relaisdesk-emailcheck | `a6c452dcbedf7f406520581c835bbc4bade8424e9e5a542c4b9f33762973a3a7` |

- `govulncheck -mode=binary` sur ces quatre exécutables : aucune vulnérabilité.

## Reste à faire

- Déployer ces exécutables sur Oracle (procédure habituelle : copie, bascule
  atomique, contrôle d'empreinte du processus actif, santé, retour arrière).
  Tant que ce déploiement n'est pas fait, l'API en production reste exposée à
  GO-2026-6859.
- La CI `go-regressions.yml` s'exécutera sur la prochaine demande de fusion.
- L'audit ne garantit pas l'absence de tout défaut ; il couvre les avis connus
  de la base Go à la date du 17 novembre 2026.
