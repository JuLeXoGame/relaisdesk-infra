# Rétablissement du suivi automatique de l'historique des connexions — plan final

## Objectif

Rétablir le suivi automatique complet de l'historique des connexions dans
l'espace client (`https://relaisdesk.fr/client/#interventions`, onglet
« Historique professionnel ») et le configurateur technicien, pour les deux
cas d'usage : postes permanents du parc et codes d'assistance temporaires
(codes viewer).

Constat central de la vérification : **tout le code prévu par le plan
d'origine est déjà en place dans le workspace** (détails en
« Contexte et état actuel vérifié »). Il ne reste que la validation
automatisée, la compilation, le déploiement et la recette en situation
réelle. Aucune modification de code n'est prévue, sauf si les tests révèlent
une régression.

## Critères de succès

1. Connexion à un poste permanent (configurateur ou portail web via
   `relaisdesk://connect/<dev-id>`) : une fiche d'intervention apparaît
   immédiatement dans l'historique avec le statut « En cours »
   (`in_progress` + `started_at` renseigné).
2. Clic « Connexion directe » sur un code viewer : la fiche passe à
   `in_progress` avec heure de début, puis est clôturée automatiquement à la
   fin de la session (`completed`, heure de fin et durée calculée, minimum
   1 minute).
3. L'onglet « Historique » du portail client recharge les données depuis le
   serveur à chaque affichage et propose un bouton « Actualiser ».
4. Aucun doublon de fiche en cas de double-clic sur « Se connecter »
   (déduplication 5 minutes).
5. Suites de tests Go vertes (`database`, `api`, `keygen`, `tests`,
   `installer/configurator`), API déployée sur le VPS Oracle, recette OK sur
   le poste `DEV-AYWM-QARC-PKWA-Q5AR`.

## Contexte et état actuel vérifié

Problématique d'origine (rappel) : les connexions aux postes permanents ne
créaient aucune fiche ; le bouton « Connexion directe » des codes viewer
lançait RustDesk sans notifier l'API (jamais `in_progress`, jamais
`completed`) ; l'onglet « Historique » ne se rafraîchissait pas.

État du code, vérifié par lecture directe (aucune exécution possible : pas
de toolchain Go dans cet environnement) :

- Base (`database/interventions.go`) : `CreateDeviceIntervention` avec
  déduplication 5 minutes sur `client_reference` (l. 176-219),
  `StartInterventionByViewerCode` (l. 222-243), `CompleteIntervention`
  amélioré — conservation du titre et de la référence existants, durée
  calculée en SQL avec minimum garanti de 1 minute (l. 126-173),
  `CancelIntervention` (l. 245-257). Les `INSERT` n'utilisent que des
  colonnes existantes : aucune migration SQL n'apparaît nécessaire.
- API : `POST /api/v1/customer/devices/{id}/connect`
  (`api/handlers/devices.go`, l. 421-441),
  `POST /api/v1/technician/devices/{id}/connect` avec contrôle d'accès
  équipe (`api/handlers/devices.go`, l. 607-634),
  `POST /api/v1/technician/viewer-codes/{code}/connect`
  (`api/main.go`, l. 364-368, vers `TechnicianConnectCodeHandler`,
  `api/handlers/technician.go`, l. 518-549),
  `POST /api/v1/technician/interventions/{id}/complete` (plus `start` et
  `cancel`, `api/handlers/interventions.go`, l. 48-90, routés par
  `api/main.go`, l. 356-363).
- Configurateur (`installer/configurator/`) : `technicianConnectDevice`,
  `technicianConnectViewerCode`, `technicianCompleteIntervention`,
  `technicianCancelIntervention` (`api.go`, l. 385-438) ;
  `launchTrackedIntervention` + pont local `interventionBridge`
  (`intervention_tracking.go`, l. 243-252 et l. 125-177 : `connected` puis
  `closed` → clôture, `closed` sans `connected` → annulation, second `closed`
  rejeté en 503 pour forcer le retry) ; appelés depuis la confirmation de
  connexion au poste permanent (`fleet_gui.go`, l. 106-108), le bouton
  « Connexion directe » des codes viewer (`gui.go`, l. 1935-1937), la fiche
  poste (`gui.go`, l. 1697-1699), le flux URI Windows
  (`gui.go`, l. 710-714, via `requestFleetConnection`), le menu et le flux
  URI Linux (`gui_linux.go`, l. 581-583 et l. 679-681) ; parsing
  `relaisdesk://connect/<dev-id>` (`main.go`, l. 14-19,
  `fleet_uri.go`).
- Espace client web : `POST .../customer/devices/{id}/connect` au clic
  « Se connecter » (`relaisdesk/client/app.js`, l. 1531),
  `fetchInterventions()` appelée à chaque affichage de l'onglet Historique
  (`app.js`, l. 853-860 et l. 2013-2014), bouton « Actualiser » câblé
  (`relaisdesk/client/index.html`, l. 456, et `app.js`, l. 2058).
- Tests : `TestDeviceConnectionInterventionTracking` et
  `TestViewerCodeConnectInterventionTracking`
  (`api/handlers/devices_test.go`, l. 562 et l. 668) couvrent
  connect → complete → listage ; tous les tests utilisent `t.TempDir()`
  (aucune écriture dans le workspace). Fichiers
  `intervention_tracking_test.go` et `fleet_api_test.go` présents côté
  configurateur (contenu non relu ici).

Reste à faire : exécuter les tests, compiler, déployer (API Oracle, web
OVH), distribuer le configurateur, recetter en situation réelle.

## Contraintes et non-objectifs

- Contraintes : ne pas casser les flux existants (wake, update, dossiers,
  équipes) ; déploiement API en premier (les clients appellent les nouvelles
  routes) ; sauvegarde SQLite du VPS avant tout redémarrage ; compatibilité
  ascendante — un ancien configurateur doit continuer à lancer RustDesk même
  sans tracking.
- Non-objectifs : portail web technicien (`relaisdesk/technicien/`, aucun
  bouton de connexion directe — hors périmètre du plan d'origine) ; refonte
  de l'UI Historique ; modification de la fenêtre de déduplication ;
  évolution du moteur RustDesk patché (supposé fournir les events
  `connected`/`closed`, voir questions ouvertes) ; nouvelles routes API.

## Décisions clés

- D1 — Réutiliser les colonnes existantes de `interventions`, sans migration
  SQL : les `INSERT`/`UPDATE` lus n'utilisent que des colonnes existantes,
  ce qui rend le déploiement VPS sans risque de schéma.
- D2 — Conserver la déduplication 5 minutes sur `client_reference` (déjà
  codée) : elle couvre le double-clic. Limite connue : deux postes
  partageant le même alias se dédupliquent entre eux ; non bloquant, à
  documenter plutôt qu'à corriger dans ce plan.
- D3 — Clôture pilotée par les events du moteur RustDesk patché via le pont
  local, pas par attente du processus launcher (RustDesk peut déléguer à une
  instance existante et quitter aussitôt — commentaire en tête de
  `intervention_tracking.go`).
- D4 — `closed` sans `connected` → annulation de la fiche (évite les fiches
  « En cours » fantômes quand la session n'a jamais abouti).
- D5 — Échec silencieux côté portail client (`catch (_) {}` autour du
  `POST .../connect`, `app.js`, l. 1532) : la connexion RustDesk reste
  possible même API injoignable, mais la fiche n'est alors pas créée.
  Comportement conservé ; documenté comme limite.
- D6 — Ordre de déploiement : API (VPS Oracle) → web (OVH) → configurateur.
  Les nouvelles routes doivent exister avant que les clients les appellent.

## Approche recommandée

Ne plus coder : valider, compiler, déployer dans l'ordre D6, recetter.
Le travail se découpe en 6 phases séquentielles (détail en « Plan de
travail »), chacune avec sa preuve de validation. Si une phase de tests
échoue, corriger le code fautif puis reprendre à la phase 0 — mais aucune
correction n'est anticipée.

## Plan de travail

- Phase 0 — Tests automatisés (surface : `database/`, `api/`,
  `installer/configurator/`, `keygen/`, `tests/` ; dépendance : machine avec
  toolchain Go). Lancer `make test` depuis la racine, puis les tests du
  configurateur (non couverts par le `Makefile`) :
  `cd installer/configurator && go test ./...`. Preuve : sorties vertes.
- Phase 1 — Compilation (dépendance : phase 0 verte). `make build` depuis
  la racine ; cross-compilation `cd api && GOOS=linux go build -o api-linux
  .` ; compilation du configurateur Windows (et Linux si utilisé en
  production — voir questions ouvertes). Preuve : binaires produits,
  `gofmt -l` et `go vet` propres sur les paquets touchés.
- Phase 2 — Déploiement API sur le VPS Oracle (dépendance : phase 1 ;
  surface : binaire `api-linux`, service `relaisdesk-api.service`).
  Sauvegarder la base SQLite distante, transférer le binaire
  (`pscp`), redémarrer le service, vérifier les logs et une route de santé.
  Preuve : service actif, `POST .../technician/devices/{id}/connect` répond
  200 sur un poste de test.
- Phase 3 — Transfert web OVH (dépendance : phase 2 ; surface :
  `relaisdesk/client/app.js`, `relaisdesk/client/index.html`). À exécuter
  uniquement si ces fichiers ne sont pas déjà en production (voir questions
  ouvertes). Conserver une copie des fichiers remplacés pour rollback.
  Preuve : bouton « Actualiser » visible dans l'onglet Historique en
  production, clic « Se connecter » suivi d'une fiche « En cours ».
- Phase 4 — Distribution du configurateur (dépendance : phase 2). Publier
  le binaire recompilé via le canal habituel (vérifier au passage que le
  moteur RustDesk embarqué/pairé est la version patchée émettant les events
  d'intervention). Preuve : version distribuée installée sur un poste
  technicien de test.
- Phase 5 — Recette en situation réelle (dépendance : phases 2-4).
  1. Connexion au poste `DEV-AYWM-QARC-PKWA-Q5AR` depuis le configurateur :
  vérifier en base distante `status = 'in_progress'` + `started_at`,
  fermer la session, vérifier `completed` + durée ≥ 1.
  2. Double-clic « Se connecter » : une seule fiche (déduplication).
  3. Code viewer : création → `client_ready` à l'ouverture client →
  « Connexion directe » → `in_progress` → fin de session → `completed`.
  4. Espace client : fiche visible immédiatement dans `#interventions`,
  badge « En cours », rafraîchissement d'onglet + bouton « Actualiser ».
  Preuve : relevés SQL + captures avant/après.

## Plan de validation

- Automatisé (machine avec Go) : `make test` (racine) ; puis
  `cd installer/configurator && go test ./...` ; `gofmt -l database api
  installer/configurator` (attendu : vide) ; `go vet` sur les trois
  paquets. Résultat attendu : 100 % vert, en particulier
  `TestDeviceConnectionInterventionTracking` et
  `TestViewerCodeConnectInterventionTracking`.
- Compilation : `make build`, `GOOS=linux go build` pour `api-linux`,
  builds configurateur Windows (+ Linux). Résultat attendu : succès sans
  avertissement nouveau.
- VPS Oracle : `systemctl status relaisdesk-api` actif après redémarrage ;
  `sqlite3 <base> "SELECT intervention_id,status,started_at FROM
  interventions ORDER BY id DESC LIMIT 3;"` conforme après chaque scénario
  de phase 5 ; test `POST .../connect` → 200 + `intervention_id`.
- Web OVH : contrôle manuel — bouton « Actualiser » présent et fonctionnel,
  fiche « En cours » après connexion, export CSV inchangé.
- Étape la plus risquée : phase 2 (redémarrage du service API en
  production). Mitigation : sauvegarde SQLite préalable + binaire précédent
  conservé pour rollback immédiat.

## Risques / Rollback

- Fiches bloquées en `in_progress` si le moteur RustDesk des postes
  techniciens n'émet pas les events `connected`/`closed` : vérifier la
  version patchée en phase 4 ; à défaut, clôture manuelle via
  `POST .../interventions/{id}/complete` ou `/cancel`.
- Fiche manquante silencieuse si l'API est injoignable depuis le portail
  client (D5) : limite acceptée ; détectable par comparaison avec les logs
  de connexion RustDesk.
- Déduplication croisée entre postes partageant le même alias (D2) :
  renommer les alias en doublon si observé.
- Tests non exécutables dans l'environnement actuel (pas de Go) : la phase
  0 doit impérativement passer sur la machine de build avant tout
  déploiement.
- Rollback API : restaurer le binaire précédent + redémarrer le service ;
  restaurer le backup SQLite uniquement en cas de corruption avérée
  (sinon, perte des écritures post-déploiement). Rollback web : re-transférer
  les copies conservées. Rollback configurateur : republier la version
  précédente.

## Questions ouvertes

1. `relaisdesk/client/app.js` et `index.html` sont-ils déjà transférés sur
   OVH en production ? (Détermine si la phase 3 est nécessaire.)
2. Le moteur RustDesk patché (events d'intervention vers le pont local) est-il
   déployé sur les postes techniciens ? (Sans lui, la clôture automatique ne
   fonctionne pas.)
3. Quelle fenêtre de déploiement pour le VPS Oracle, et les accès
   `plink`/`pscp` sont-ils disponibles depuis la machine d'exploitation ?
4. Le configurateur Linux est-il utilisé en production, ou Windows
   uniquement ? (Détermine les cibles de compilation de la phase 1.)

## Réponses de l'exploitant (2026-09-20) et ajustements

1. Oui : les fichiers client sont déjà sur OVH — mais les versions locales
   (avec le nouveau code) doivent y être re-transférées (phase 3 maintenue).
2. Les fichiers de `relaisdesk/downloads/` sont sur Oracle et à jour : le
   moteur RustDesk distribué est donc la version patchée attendue.
3. `plink`/`pscp` disponibles : le déploiement passe par ces outils.
4. Pas encore de clients : pas de fenêtre de déploiement à négocier, tout
   passe en production sans précaution commerciale particulière.

Constat complémentaire après vérification : les sources du suivi
d'intervention (15h23-16h33) sont plus récentes que tous les binaires
(`downloads/` construits à 03h18-03h19, `api-linux` à 13h20), et les
binaires du configurateur ne contiennent pas les littéraux du nouveau code
(`/connect`, `INT-`, `intervention_id`). **La recompilation est donc
obligatoire** pour l'API comme pour le configurateur (4 artefacts
technicien : Portable, Linux, .deb, Setup ; les binaires viewer sont à jour
et inchangés).

Exécution : l'environnement de l'assistant ne permet ni compilation ni
transfert (pas de Go exécutable, interopérabilité Windows/WSL en panne,
aucun accès réseau). La mise en production est donc livrée sous forme de
runbook PowerShell clé en main, à exécuter sur le poste d'exploitation :
`scripts/deploy-interventions-20260920.ps1 -VpsTarget "ubuntu@<IP_ORACLE>"`.
Il enchaîne phases 0→5 (tests, builds API+configurateur, signature du
manifeste, déploiements Oracle, pause pour le transfert OVH manuel avec
vérification automatique, guide de recette).

## Déploiement effectué (2026-09-20)

Runbook exécuté avec succès par l'exploitant :
- Phase 0 : tests verts (`database`, `keygen`, `api`, `tests`,
  `installer/configurator`) ; avertissements `gofmt` pré-existants sans
  gravité.
- Phase 1 : API Linux + 3 outils compilés ; configurateur reconstruit
  (Portable, Linux, .deb, Setup NSIS) ; manifeste re-signé.
- Phase 2 : API déployée, `healthy`/`connected`, processus vérifié sur le
  nouveau binaire (`exe_sha=41a67100…7ef23`).
- Phase 4 : 9 fichiers downloads synchronisés sur Oracle, manifeste public
  vérifié (les 3 binaires viewer locaux, plus récents, ont complété la
  synchro).
- Phase 3 : fichiers OVH vérifiés en ligne (marqueurs présents).

Correctifs appliqués en cours de route :
- `installer/configurator/rustdesk_darwin.go` : bug de syntaxe
  `tomlEscapeDarwin` (littéraux sans guillemets, invisible hors macOS) +
  test `TestTomlEscapeDarwin`.
- Runbook : `SHA256SUMS.txt` en LF, `mkdir -p` distant avant `pscp`,
  sauvegardes rollback non-écrasantes, `-UseBasicParsing`, contrôle
  syntaxique strict des fichiers darwin.

Reste : recette fonctionnelle manuelle (phase 5) — connexion au poste
`DEV-AYWM-QARC-PKWA-Q5AR`, scénario code viewer, contrôle de l'onglet
Historique client.
