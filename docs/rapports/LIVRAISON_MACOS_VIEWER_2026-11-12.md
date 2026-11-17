# Livraison des Viewer macOS — 12 novembre 2026

Les deux disques du Viewer macOS (Apple Silicon et Intel) ont été remplacés sur
le serveur Oracle de production. Les autres fichiers publiés sont inchangés.
Le numéro produit reste 1.0.0. OVH n'est pas concerné par ce déploiement.

## Pourquoi

Correction de l'enregistrement temporaire du Viewer macOS : attente de
l'enregistrement, annonce avec contexte, prise en compte de `key_confirmed` et
messages d'interface associés (installer/viewer : `api.go`, `fleet.go`, `gui.go`,
`gui_linux.go`, `i18n.go`, `rustdesk_darwin.go`, `rustdesk_windows.go` et le test
`rustdesk_darwin_test.go`). Le détail exact figure dans le commit ci-dessous.

## Sources

- Dépôt `relaisdesk-infra`, commit
  `46baa8aef398a160dedb5b8d4f097ac010acad25` « Fix temporary macOS viewer
  registration », poussé sur la branche `codex/security-perf-1-0-20261023`.
  Ce commit n'est pas fusionné dans `main`.
- Exécution GitHub Actions « Package verified macOS engine 1.0.0 », nº
  `43276292293`, réussie, créée le 2026-11-12T18:06:01Z :
  <https://github.com/JuLeXoGame/relaisdesk-infra/actions/runs/43276292293>.
  Les deux jobs (arm64 sur `macos-15`, amd64 sur `macos-15-intel`) sont en succès.
- Le workflow `.github/workflows/package-macos-verified-engine.yml` a été modifié
  par le même commit. Il ne reconstruit pas le moteur : il récupère le moteur
  `RelaisDeskEngine.app` contenu dans les DMG Viewer de l'exécution `40838090440`
  (commit `0d7a407`, 29 octobre), après contrôle de l'empreinte de ce DMG et de
  celle de l'exécutable du moteur, puis `codesign --verify --deep --strict`.
  Il exécute les tests et `go vet` des lanceurs (viewer, configurator), puis
  `scripts/package-macos-verified-engine.sh` assemble et vérifie les applications
  et les DMG. Seul le lanceur du Viewer est donc recompilé ; le moteur est celui du
  29 octobre.

| Architecture | DMG Viewer d'origine (29 oct.) | Exécutable du moteur réutilisé |
| --- | --- | --- |
| ARM64 | `11bbe50f397e83a3fa74fd5b1deb48c27d1abaf7a3119b4fc2ccb609dd19a16d` | `be486bb93e77f91d1400978e5e9841da396e2529f3f209c5a3c8287055ca9e37` |
| Intel | `d5cf83000ebf9c923c910988208501e02b3754dfed4ec8a5856cdce935b743bb` | `7c7a7ca074b7130a5aad5f5364898fe20cc9687d243c8b77a666929e4f1968b8` |

Ces empreintes du moteur sont celles du rapport du 29 octobre.

## Fichiers livrés

| Fichier | Taille (octets) | SHA-256 |
| --- | --- | --- |
| `RelaisDesk_Mac.dmg` | 41285340 | `ca7351ba68b5a41f4f872dbfb7ea93c0d99dfda11d7de3e7867582c0215aa52e` |
| `RelaisDesk_Mac_Intel.dmg` | 45470433 | `3f546f09ed3752150718ee7dbc052246a608a269259f5e9c23d7a54eb483b699` |

Les fichiers sont datés du 2026-11-12 18:27 UTC dans `/opt/relaisdesk/downloads`.
`SHA256SUMS.txt` et `release-manifest.json` ont été actualisés le même jour
(manifeste : `published_at` 2026-11-12T18:25:50Z, version 1.0.0 ; SHA-256 du
manifeste préparé localement :
`a6d4e7b04587eda6ba983a0f5c3d4076a4da588988072457163d34502620807c`).

Inchangés depuis le 29 octobre : les deux DMG Technicien (leurs versions
reconstruites par la même exécution ont été téléchargées dans `.cache` mais pas
publiées), tous les fichiers Windows et Linux, ainsi que l'API et le serveur.

## Vérifications avec preuves

- Exécution GitHub `43276292293` réussie sur les deux architectures, y compris les
  étapes de tests des lanceurs, de récupération du moteur vérifié et de
  construction et vérification des applications et DMG (consulté via l'API GitHub).
- Le 17 novembre 2026, sur Windows : `go vet` et `go test -tags ci ./...` dans
  `installer/viewer`, réussis. Le workflow de non-régression Go n'a pas été exécuté
  sur le commit `46baa8a`.
- Artefacts GitHub téléchargés (`.cache/macos-20261112/macos-actions-*.zip`) avec
  contrôle de l'empreinte de l'archive et du commit producteur par
  `stage.py`, puis extraits. Les copies locales des deux DMG de
  `.cache/macos-20261112/downloads/` ont les empreintes ci-dessus (recalculées).
- Le script de publication (`.cache/macos-20261112/publish.sh`), dont le
  remplacement sur Oracle et le dossier de retour arrière sont constatés par
  l'exploitant, prévoit : contrôle de
  l'empreinte du manifeste précédent (celui du 29 octobre), copie par liens
  durs, remplacement des seuls deux DMG, de `SHA256SUMS.txt` et du manifeste,
  vérification de la version et du jeu des fichiers modifiés, puis échange
  atomique des dossiers avec contrôles HTTP locaux et retour arrière automatique
  en cas d'échec. Le journal de sortie de cette exécution n'est pas conservé
  dans le dépôt consulté ; l'état final est établi par le contrôle suivant.
- Contrôle public archivé (`.cache/macos-20261112/public-verified.json`) : API en
  bonne santé, version 1.0.0, empreintes des deux DMG conformes, accès anonyme aux
  deux DMG en HTTP 403 (protection bêta conservée).
- Un script en lecture seule (`.cache/oracle-readonly-mac-20261112.sh`) permet de
  relire sur Oracle l'empreinte du manifeste et l'état du service API ; son
  résultat n'est pas conservé, il ne prouve donc rien en lui-même.
- Un binaire de test Go Linux (`.cache/viewer-linux-20261112-amd64.test`) est
  présent ; il prouve seulement qu'un binaire de test a été compilé le 12
  novembre, pas qu'il a réussi.

## Non vérifié

- Aucune session distante complète entre deux postes avec les nouveaux Viewer
  macOS n'est documentée. Le correctif d'enregistrement n'est donc pas confirmé
  en conditions réelles.
- Aucune installation ni ouverture des DMG sur un Mac physique n'est documentée
  en dehors des machines de l'intégration continue.
- Le manifeste n'est pas revérifié ici par une seconde implémentation
  indépendante pour le 12 novembre ; la signature Ed25519 est produite par
  `sign-downloads.ps1` et contrôlée par `verify-release.py` pendant la
  publication, sans journal conservé consulté.
- Les empreintes complètes des fichiers installés sur Oracle ne sont pas
  relues dans ce rapport. Le contrôle HTTP public archivé ne porte que sur le
  statut anonyme et les empreintes du manifeste.
- Les signatures macOS restent ad hoc ; aucune notarisation n'est revendiquée.

## Retour arrière et conséquences

- Retour arrière privé : `/opt/relaisdesk/deployments/macos-20261112-PUFt0eUB`
  (contient `downloads.previous`, l'ancien dossier des téléchargements avec les
  DMG et le manifeste du 29 octobre).
- Le produit reste en version 1.0.0 : un client macOS déjà installé qui compare
  seulement le numéro de version ne détectera pas la mise à jour. Les Mac déjà
  équipés doivent remplacer manuellement l'application par les nouveaux DMG ;
  vérifier la date et l'empreinte du fichier.
- Le commit `46baa8a` n'est pas fusionné dans `main` : la branche
  `codex/security-perf-1-0-20261023` reste la référence des sources publiées.

## Site

Le fichier `relaisdesk/logiciel-libre.html` est modifié directement dans le
dossier du site : nouvelle première ligne du tableau « Viewer macOS —
reconstruction du 12 novembre 2026 » avec lien vers le commit `46baa8a`, date de
mise à jour et paragraphe des reconstructions actualisés.

À transférer sur OVH : `relaisdesk/logiciel-libre.html`. La page publique
actuelle date du 23 octobre ; ce fichier local (dernière modification des
contenus du 29 octobre, puis ce correctif) est la référence à envoyer, et il
rattrape aussi la mise à jour d'octobre. `relaisdesk/downloads/LIRE-MOI.txt` du
29 octobre n'est pas modifié ici. Les fichiers de `relaisdesk/downloads/` ne sont
pas touchés ; leur alignement sur le lot du 12 novembre n'est pas vérifié ici.
Aucun transfert OVH, ni archive ZIP, ni commit n'a été effectué.
