# Correctif des dossiers de l'équipe — Technicien Windows

Date : 26 octobre 2026. Version produit conservée : **1.0.0**.

## Cause et correction

L'écran Équipe utilisait le catalogue personnel du client, alors que l'écran Parc
utilise les dossiers de la licence Technicien. Ces catalogues pouvaient donc
présenter des noms différents. Le panneau Équipe conservait aussi ses données
en mémoire lors des changements d'écran.

Le Technicien charge maintenant le même catalogue que le parc de la licence
concernée. Le dialogue recharge les noms à chaque ouverture et affiche le chemin
des sous-dossiers. L'invitation d'un utilisateur filtre les dossiers par licence
et réinitialise la sélection lors d'un changement de licence.

Les droits existants sont conservés. Si un dossier déjà autorisé n'est plus
disponible dans le catalogue, son identifiant reste visible et coché pour éviter
une révocation involontaire. L'indication sur une sélection vide devient
« aucun accès », conformément au comportement du serveur.

L'API accepte un paramètre facultatif `license_id` sur le catalogue client. Elle
contrôle la propriété de la licence avec l'identité de la session avant toute
lecture des dossiers. Une licence étrangère ou inconnue renvoie 404, une requête
mal formée 400 et l'absence de session 401. Les appels sans ce paramètre gardent
leur comportement existant. Aucune migration ni modification des droits en
base n'est nécessaire.

## Validation

- API Linux : suite complète `go test ./...` et `go vet ./...` validée.
- Technicien Windows : suite complète du module et `go vet` validés avant la
  compilation native CGO et l'assemblage NSIS, avec Go 1.26.10.
- Tests de régression : correspondance avec le catalogue Technicien, noms après
  renommage, hiérarchie, plusieurs licences, isolation entre clients, refus des
  anciens catalogues non filtrés et conservation des droits devenus indisponibles.
- L'extraction de l'installateur retrouve le même exécutable que le portable.
- Les treize artefacts du manifeste sont contrôlés par SHA-256 et par la
  signature Ed25519 officielle. Seuls les deux programmes Technicien Windows
  changent ; les dix autres programmes conservent leurs empreintes.
- Le contrôle `node scripts/fleet-legal-checks.mjs` passe après l'ajout des
  références aux sources du correctif sur la page locale du site.

## Binaires et sources

| Élément | SHA-256 |
| --- | --- |
| API Linux | `46d2201bb2c35932b34d986a4e1faca3814d32e988e0bd5df2c90d32c683a370` |
| Technicien Windows portable | `557e4cd77ec37385b09f8cde4affcf23237e0d11e83881a4ccb344c1d199af86` |
| Installateur Technicien Windows | `bf7b4dca166580ade7fae6201ff991ed059de52599fd623d6e009bbd5632854d` |
| Manifeste signé | `8f9246df053aa62054c9eaeea839b09947026afdd14fcd52cba68c7a5b51d073` |

Le manifeste conserve `version=1.0.0` et porte la date de publication
`2026-10-26T11:40:12Z`. Le moteur Rust Windows est inchangé.

Les sources corrigées sont publiées dans `JuLeXoGame/relaisdesk-infra` au commit
`af47dab659e916d69223c95f6fb94a6c0e4056a7`. La documentation de reconstruction et
les références du site sont publiées au commit `68f2207`, sur la branche autorisée
`codex/security-perf-1-0-20261023`.

## Déploiement Oracle effectué

L'API et les téléchargements Windows corrigés sont en service. Les empreintes
des quatre fichiers actifs ci-dessus correspondent aux fichiers locaux vérifiés.
La configuration `api.env` est inchangée et les services API et Nginx sont actifs.

Sauvegardes de retour arrière conservées sur Oracle :

- API précédente et sauvegarde SQLite cohérente, dont l'intégrité a été contrôlée :
  `/opt/relaisdesk/deployments/team-folders-api-20261026-nceCaWuz/` ;
- Téléchargements précédents et copie de la configuration :
  `/opt/relaisdesk/deployments/team-folders-downloads-20261026-tLvaBcdh/`.

Les scripts de déploiement prévoient un retour au binaire ou au lot précédent
en cas d'échec, sans restaurer une ancienne base sur des écritures récentes.
Les transferts ont été terminés par blocs puis reconstitués et contrôlés dans
le dossier privé de préparation avant la bascule publique.

Vérifications publiques après déploiement : santé HTTP 200, base connectée,
version 1.0.0, manifeste égal au fichier signé attendu et réponses HTTP 206 avec
les bonnes tailles pour les deux programmes Windows. Le catalogue client et
l'administration restent protégés par HTTP 401 sans session ; l'accès bêta
macOS conserve son refus HTTP 403. Les résultats sont consignés dans
`.cache/team-folders-public-verification.json`.

Téléchargements publiés :

- [Installateur Technicien Windows 1.0.0](https://api.relaisdesk.fr/api/v1/downloads/RelaisDesk_Technicien_Setup_1.0.0.exe)
- [Technicien Windows portable](https://api.relaisdesk.fr/api/v1/downloads/RelaisDesk_Technicien_Portable.exe)

## Fichiers locaux

Les programmes courants dans `installer/build/`, `bin/api`,
`installer/build/api-linux` et l'entrée API de `bin/SHA256SUMS.txt` sont actualisés.
Les anciens fichiers de compilation sont conservés dans le dossier privé de
travail `.cache/team-folders-20261026/local.previous/`.

Référence du site modifiée directement, sans archive de livraison :

- `relaisdesk/logiciel-libre.html` ;
- `relaisdesk/downloads/RelaisDesk_Technicien_Portable.exe` ;
- `relaisdesk/downloads/RelaisDesk_Technicien_Setup_1.0.0.exe` ;
- `relaisdesk/downloads/SHA256SUMS.txt` ;
- `relaisdesk/downloads/release-manifest.json`.

OVH reste hors du déploiement. Ces cinq fichiers constituent les changements
locaux à transférer ultérieurement sur OVH.

Pour utiliser le correctif, fermer l'ancienne application Technicien puis
installer ou lancer le nouveau programme Windows. Le numéro 1.0.0 étant conservé,
le remplacement manuel permet de distinguer ce correctif de l'ancienne version.
