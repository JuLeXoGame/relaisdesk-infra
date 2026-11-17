# Déploiement de l'API en Go 1.26.11 — 17 novembre 2026

Déploiement terminé et vérifié sur Oracle. Version produit inchangée : **1.0.0**.
OVH et les conteneurs hbbs/hbbr ne sont pas concernés.

## Motif

Correction de [GO-2026-6859](https://pkg.go.dev/vuln/GO-2026-6859) (plantage du
serveur HTTP/1 lié à `MaxBytesReader`, corrigé en Go 1.26.11). Voir la
[note d'audit](../audits/GO_1.26.11_2026-11-17.md) : tests, empreintes et
résultats `govulncheck` des exécutables.

## Exécutables déployés

Go 1.26.11, compilation Linux sans CGO, chemins supprimés et symboles réduits.

| Exécutable | SHA-256 |
| --- | --- |
| api | `9b5583f1b687a1b357b7589c5d6f30be65225ac2eddd6b7b82fc72e7381b18ea` |
| relaisdesk-backup | `2fb68546d40dba8bd0e528f9b5f6cdd43e5838e67f3ad8b787c04edc2262ad5f` |
| relaisdesk-dbcheck | `8d6e5946b52fce7f93cac6429548fdf36072f80d5f93ff13fb15382b9be30f27` |
| relaisdesk-emailcheck | `a6c452dcbedf7f406520581c835bbc4bade8424e9e5a542c4b9f33762973a3a7` |

Exécutable remplacé : api `d1b4977d692e92e09718131c788407d8ef8f7bd3469f67f59d1eb4e4429249ec`
(déploiement du 29 octobre).

## Procédure

Script `scripts/deploy-followup-api-20261117.sh`, dérivé de celui du 23 octobre
(seuls les chemins datés changent). Le transfert a été fait par SSH dans un dossier
de préparation, avec contrôle SHA-256 avant et pendant la bascule.

1. Vérification que l'exécutable installé et le processus actif correspondent à
   l'empreinte précédente.
2. Contrôle préalable de l'outil de migration sur une copie SQLite : intégrité,
   clés étrangères et nombre de lignes inchangés (`PREFLIGHT_OK`).
3. Sauvegarde cohérente de la base, arrêt de l'API, remplacement atomique des
   quatre exécutables, redémarrage.
4. Contrôles : santé (version 1.0.0, base connectée), intégrité SQLite, empreinte
   du processus actif, empreinte des quatre fichiers installés, tarifs et essais
   publics identiques avant/après, fichiers de configuration et téléchargements
   inchangés, WireGuard et conteneurs hbbs/hbbr non redémarrés.

Résultat : `DEPLOY_OK`, code de sortie zéro. Données métier identiques avant et
après (1 licence, 2 postes, 1 commande, 7 comptes). `NRestarts=0`.

Retour arrière privé :
`/opt/relaisdesk/deployments/followup-api-20261117-6RTI9o0b` (exécutables
précédents et sauvegarde de la base d'avant bascule). Ne pas restaurer
automatiquement la base : cela effacerait les écritures postérieures à la bascule.

## Contrôles depuis l'extérieur

- Santé API : HTTP 200, version 1.0.0, base connectée.
- Tarifs publics et manifeste des versions : HTTP 200.
- Administration (`/api/v1/admin/licences`, factures) et espace client
  (`/api/v1/customer/device-folders`, `service-billing`) anonymes : HTTP 401.
- Ancien domaine `api.informatiqueadomicile03.fr` : santé HTTP 200.
- Ports TCP 21115, 21116 et 21117 joignables.
- Processus actif : empreinte `9b5583f1…`. Conteneurs hbbs/hbbr actifs, aucun
  redémarrage automatique (ils tournent depuis le redémarrage du serveur de
  17:05 UTC, antérieur à ce déploiement).
- Aucun avertissement dans le journal de l'API depuis la bascule.

## Limites

- Les sauvegardes quotidiennes utiliseront le nouvel outil dès la prochaine
  exécution (planifiée vers 03:30 UTC) ; elle n'a pas encore tourné avec lui.
- Les programmes distribués (clients, serveur hbbs/hbbr) ne sont pas modifiés.
- Cette passe ne corrige pas les avis Rust ouverts (`libsqlite3-sys` dans hbbs).
- Les tests de contrôle ne remplacent pas un test de charge.
