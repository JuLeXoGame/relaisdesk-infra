# Déploiement RelaisDesk 1.0.0 — 29 octobre 2026

Déploiement terminé : API, serveur et douze programmes publiés et vérifiés.
OVH exclu conformément à la demande. Le numéro produit reste 1.0.0.

## Sources

- Infrastructure fusionnée : `de0b6b5a912b94c4c16567f167fb8a29d30583bc`.
- Moteur client fusionné : `a5043e4ed97c80412d25c39a0d91b8be4c402be4`.
  Le code applicatif local Windows/Linux (`421877211d56733097ae5585db25d92386434ed4`)
  et celui fixé pour macOS (`2c074a088c4980562a66d2788c7cdd744f4b0211`)
  sont identiques ; les différences concernent la CI, ses tests et outils.
- Serveur fusionné : `b5934ee107c26035877119da195bf04746804944`.
- Correction de compilation macOS : `0d7a40722967dd3b493af1799e03e171588f3d31`,
  ajout de `rustfmt` à Rust 1.99.1 ; exécution `40838090440`.
- Instructions publiques : `docs/RECONSTRUCTION_1.0.0_2026-10-29.md`.

## API et outils Oracle — terminé

Go 1.26.10, compilation Linux sans CGO, chemins supprimés et symboles réduits.

| Exécutable | SHA-256 |
| --- | --- |
| api | `d1b4977d692e92e09718131c788407d8ef8f7bd3469f67f59d1eb4e4429249ec` |
| relaisdesk-backup | `cbe491074ef615ea237e78747fe75076fed41df023254a166a195957d454fe7c` |
| relaisdesk-dbcheck | `7a8263a564f16d7d5b4362b96e1b2aaf82f184c3cca3f5386c82ede37b11fcd7` |
| relaisdesk-emailcheck | `da9d91df8ad35240516208c69399d96e91d5724480b233e3a373560c17016518` |

Migration préalable sur copie SQLite, sauvegarde cohérente avant bascule,
intégrité et clés étrangères vérifiées. Tarifs, essais et données métier
conservés. L'empreinte du processus actif correspond à celle du nouvel exécutable.
API saine en version 1.0.0, base connectée, zéro redémarrage automatique.

Retour arrière privé :
`/opt/relaisdesk/deployments/followup-api-20261029-CBhD66Ya`.

## Serveur hbbs/hbbr — terminé

Les 27 tests Rust réussissent ; les exécutables de production sont reconstruits
avec Rust 1.99.1. Nouvelle image :
`relaisdesk/rustdesk-server:followup-20261029-amd64`, identifiant
`sha256:0ab2c03df6c0b532fd3b562feddfd514b674c92cc4d3b2ff0743544989abe029`.

Base Debian bookworm-slim fixée par digest et paquets d'exécution actualisés.
Conteneurs actifs, utilisateur 10001, racine en lecture seule, zéro redémarrage
automatique. Authentification obligatoire et clés conservées. Les empreintes des
exécutables actifs correspondent aux fichiers construits. Les ports attendus
écoutent. L'intégrité SQLite et les clés étrangères restent valides.
Nginx et WireGuard sont actifs ; leurs configurations n'ont pas été modifiées.

Retours arrière privés :
`/opt/relaisdesk/deployments/followup-server-20261029-lCjL0FZ6` et
`/opt/relaisdesk/deployments/followup-sources-20261029-kXpiqhag`.
La référence serveur de l'API pointe sur `b5934ee`.

## Programmes — contrôles déjà effectués

Windows : moteurs reconstruits, tests et analyse statique des deux lanceurs
réussis. Les installateurs NSIS sont extraits sans installation ; leurs charges
utiles correspondent exactement aux exécutables portables vérifiés.

Linux : bibliothèque native reconstruite, AOM SVC testé, `ldd -r` sans dépendance
manquante ni symbole non résolu. Les deux lanceurs sont construits, leurs tests
exécutés avec succès et leurs paquets Debian contrôlés : amd64, version 1.0.0,
exécutables identiques aux fichiers autonomes. Le démarrage graphique du
Technicien (1020 × 640) et du moteur Flutter (800 × 600) réussit sous Xvfb dans
des espaces de réseau et de processus isolés, avec des profils de test distincts.
Ces contrôles ne constituent pas une session distante complète entre deux postes.

Outils locaux privés et exécutables de `bin/` et `installer/build/` actualisés,
avec copie préalable dans `.cache/deploy-20261029/local-outputs.previous/`.
Les générateurs de clés n'ont pas été exécutés. Aucun outil d'administration
ni aucune clé privée n'est publié dans les programmes à télécharger.

macOS : exécution GitHub `40838090440` réussie pour ARM64 et Intel. Les tests Go,
audits Rust, bornes SVC AV1, architectures, signatures ad hoc, contenu des
applications complètes et des DMG montés passent sur les deux machines Apple.
Les quatre messages `MACOS_PACKAGE_VERIFIED` sont conservés dans
`.cache/deploy-20261029/macos-completed.log`. Empreintes des exécutables natifs :
ARM64 `be486bb93e77f91d1400978e5e9841da396e2529f3f209c5a3c8287055ca9e37`,
Intel `7c7a7ca074b7130a5aad5f5364898fe20cc9687d243c8b77a666929e4f1968b8`.

La demande infrastructure nº 2, limitée à l'outil de compilation manquant et aux
références de sources, est fusionnée au commit
`91b5755beae5638b66dedae43f704c204e878fe1`. Les tests et audits de ses huit modules
Go réussissent avant fusion (`40840051540`) et sur main (`40840922763`).
## Publication et contrôles finaux — terminé

Les douze programmes sont transférés par HTTPS et vérifiés par SHA-256 avant
publication. Les DMG proviennent des artefacts GitHub Actions dont le producteur,
la révision, la taille et l'empreinte de l'archive sont vérifiés. Les fichiers
extraits correspondent aux empreintes contrôlées sur les machines Apple.

Le manifeste est signé avec la clé Ed25519 officielle `release-1`, vérifié par
l'outil Go puis indépendamment par Python. Il contient les douze programmes et
`SHA256SUMS.txt`, toujours en version **1.0.0**.

- Date du manifeste : `2026-10-29T08:56:14Z`.
- SHA-256 : `3b4bad1e703e0f601bc81797147e741e5d4258c1fccbef2d7d7c684ca28d3e54`.
- Bascule atomique Oracle : résultat `DOWNLOAD_DEPLOY_OK`, code de sortie zéro.
- Retour arrière privé :
  `/opt/relaisdesk/deployments/followup-downloads-20261029-TIISJp0a`.

La référence client de l'API pointe désormais sur `a5043e4`. Les contrôles
finaux confirment les empreintes de tous les fichiers installés, l'authentification
obligatoire du serveur, l'utilisateur 10001, la racine en lecture seule, les
services actifs, zéro redémarrage automatique, l'intégrité SQLite et les clés
étrangères. Le manifeste servi publiquement est identique au manifeste vérifié.

Contrôles depuis l'extérieur : santé API HTTP 200 en version 1.0.0, base
connectée ; administration et espace client anonymes HTTP 401 ; neuf artefacts
Windows/Linux/empreintes HTTP 206, avec les tailles et premiers octets attendus ;
quatre DMG anonymes HTTP 403, protection bêta conservée. Les ports TCP 21115,
21116 et 21117 sont joignables. Les empreintes complètes ont été vérifiées sur
Oracle ; le contrôle HTTP public utilise des requêtes de plage.

Preuves locales : `.cache/deploy-20261029/oracle-final-verified.json`,
`public-verified.json`, `macos-receipt.json` et les journaux de transfert et de
signature du même répertoire.

Le brouillon temporaire GitHub nº `427310115`, resté non publié, est supprimé
après les contrôles. Sa suppression et l'absence du tag provisoire sont vérifiées
par HTTP 404. La copie SSH `.secrets/oracle-deploy-20261029` est supprimée ; les
clés SSH et de signature originales sont conservées. Aucun secret ni aucune
donnée métier n'a été transféré dans le relais GitHub.

Le site est modifié directement dans `relaisdesk/`. Fichiers de texte préparés :
`logiciel-libre.html` et `downloads/LIRE-MOI.txt`. Les douze programmes,
`downloads/SHA256SUMS.txt` et `downloads/release-manifest.json` sont actualisés
directement dans `relaisdesk/downloads/` et vérifiés une nouvelle fois ; leur
contenu correspond au lot Oracle. Aucun transfert OVH ni archive ZIP du site.
Les signatures macOS sont ad hoc ; les exécutables Windows ne
revendiquent pas de signature Authenticode. Le manifeste utilise Ed25519.

Les clients qui comparent seulement le numéro de version peuvent nécessiter un
remplacement manuel puisque le produit reste en 1.0.0. Les retours arrière
concernent les exécutables et configurations ; ne pas rétablir automatiquement
une ancienne base au détriment des écritures postérieures à la bascule.
