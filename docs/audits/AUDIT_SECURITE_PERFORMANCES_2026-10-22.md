# Audit sécurité et performances — 22 octobre 2026

**Suite :** les corrections complémentaires et leur validation sont consignées dans
[le suivi du 23 octobre](CORRECTIFS_COMPLEMENTAIRES_2026-10-23.md). Les constats
ci-dessous décrivent l'état de l'audit initial.

Revue locale de RelaisDesk, avec corrections ciblées et tests sur données synthétiques.
Révisions de départ : dépôt principal `cd611f5`, client Rust `9d361f514`, serveur Rust `6248479`.

## Résultat

Une élévation de privilèges administrateur a été reproduite et corrigée. Le traitement
des remontées du parc et les caches web ont également été corrigés. Des mises à jour
de dépendances restent prioritaires : les sources actuelles ne doivent pas être
considérées comme exemptes de vulnérabilités connues.

Cette revue couvre transversalement l'API, SQLite, les espaces web, les lanceurs,
les contrôles d'autorisation des forks, les sauvegardes, les scripts et la CI.
Elle ne constitue pas une revue exhaustive de chaque ligne des moteurs RustDesk,
de Flutter ou des bibliothèques natives. Aucun test d'intrusion ni mesure de charge
n'a été effectué sur la production. Aucun déploiement Oracle/OVH, aucune rotation
de secrets et aucune reconstruction de version distribuable n'ont été réalisés.

## Problèmes corrigés

### P1 — Un membre d'équipe pouvait hériter des droits administrateur

Dans `api/middleware/auth.go`, `ValidateAdminSession` refusait correctement les
sessions d'équipe. Le middleware essayait cependant ensuite un second chemin :
validation de la session technicien, puis lecture du rôle de sa licence. Si cette
licence était marquée `ADMIN`, ce second chemin autorisait le membre.

Le scénario exige une session d'équipe valide rattachée à une licence administrateur
éligible aux équipes ; il ne s'agit pas d'un accès anonyme. L'impact est néanmoins
élevé : les routes administrateur deviennent accessibles à un utilisateur normalement
limité aux dossiers partagés.

Le chemin de secours a été supprimé. La validation centrale, qui prend déjà en charge
les sessions issues du secret maître et celles des propriétaires administrateurs,
est désormais l'unique décision d'autorisation.

Preuve : `TestAdminAuthDoesNotPromoteTeamMember` obtenait HTTP 204 pour le membre
avant correction ; il attend et obtient maintenant 401. Les sessions légitimes du
propriétaire et du maître restent acceptées. Le secret maître brut reste refusé
sur les routes protégées.

Avant mise en service, vérifier si des membres ont été associés à des licences
`ADMIN` dans la version déployée. Si oui, examiner les actions administratives et
les comptes/licences créés pendant cette période. La revue locale ne permet pas
de conclure qu'il y a eu exploitation.

### P2 — Le quota général neutralisait le dimensionnement du parc

`api/main.go` prévoyait 6 000 remontées/minute/IP et 5/minute/poste, mais le middleware
général appliquait d'abord 300 requêtes/minute/IP à toutes ces remontées. À une
cadence de 45 secondes, environ 225 postes suffisaient à consommer ce quota en
charge régulière, sans compter les autres requêtes. C'est un calcul de seuil,
pas une mesure de capacité réelle.

Dans `api/middleware/service_meter.go`, seul le POST exact vers
`/api/v1/devices/heartbeat` utilise maintenant ses limites dédiées. Les autres
méthodes et chemins conservent le quota général. La validation cryptographique
reste effectuée par le gestionnaire de remontées.

Le test `TestFleetHeartbeatKeepsItsOwnBudgetBehindSharedIP` reproduit le blocage
avec des quotas réduits et vérifie aussi le maintien du plafond IP ainsi que
l'indépendance du budget des autres routes. Le test existant du limiteur par poste
continue de vérifier le refus de la troisième requête avec un plafond de deux.

### P2 — Lecture et rétention de données non fiables dans le limiteur

Dans `api/middleware/logging.go`, le limiteur lisait le corps avant de vérifier
le quota IP. `io.LimitReader` tronquait silencieusement les requêtes dépassant
1 Mio, et les identifiants fournis par le client étaient conservés tels quels
comme clés du cache de 10 000 entrées. De très longs identifiants pouvaient donc
entraîner une rétention mémoire disproportionnée.

Corrections :

- contrôle du quota IP avant lecture du corps ;
- rejet HTTP 413 au-delà de 1 Mio, au lieu d'une troncature ;
- rejet HTTP 400 en cas d'erreur de lecture ;
- conservation d'une empreinte SHA-256 de 32 octets pour chaque identifiant dans
  le limiteur, sans conserver l'identifiant arbitrairement long.

Les tests reproduisaient avant correction le passage des corps trop longs et des
erreurs de lecture vers le gestionnaire suivant. Ils vérifient maintenant le rejet,
y compris lorsqu'un JSON valide est suivi d'un excès d'espaces, et l'absence de
lecture du corps après dépassement du quota IP. Il ne s'agit pas d'une mesure
globale de mémoire : les allocations temporaires et le nombre de requêtes
concurrentes restent à prendre en compte dans un test de charge.

### P3 — Les applications web effaçaient leurs caches respectifs

Les service workers administraient tous les caches de l'origine en supprimant
tout nom différent du leur. Une activation de l'espace administrateur pouvait
ainsi supprimer le cache de l'espace technicien, et inversement.

`relaisdesk/admin/sw.js` et `relaisdesk/technicien/sw.js` suppriment maintenant
uniquement les anciennes versions de leur propre préfixe. Les versions de cache
passent respectivement à `v6` et `v4` pour appliquer le changement.

Le comportement a été vérifié par exécution des événements d'activation dans Node :
les anciens caches propres sont supprimés, le cache courant et ceux des autres
applications sont conservés. Le partage des caches par origine est décrit dans
la [documentation CacheStorage](https://developer.mozilla.org/en-US/docs/Web/API/CacheStorage).

### Fiabilité des tests d'administration

`tests/admin_dashboard_test.go` injectait encore des clés en clair après la migration
vers les empreintes SHA-256. Il attendait également dix accès pour Pro au lieu des
cinq du catalogue actuel, et attendait la récupération d'une clé complète via
`unmask=true` alors que les clés ne sont affichées qu'à leur création.

Les données et attentes ont été alignées sur les règles existantes. Le test vérifie
désormais que la liste ne divulgue pas la clé complète, même à l'administrateur.
Aucune règle de prix ou d'authentification de production n'a été assouplie.

## Dépendances : travaux prioritaires restant à effectuer

Les scanners ont consulté leurs bases publiques le 22 octobre 2026. Les résultats
bruts sont conservés dans `.cache/audit-go-api-20261022.json`,
`.cache/audit-rust-server-20261022.json` et `.cache/audit-rust-client-20261022.json`.
Les fichiers `go.mod`, `go.sum` et `Cargo.lock` n'ont pas été modifiés par cet audit.

### Go

Le projet sélectionne Go **1.26.6** ; le Go installé globalement est 1.26.5.
Le scan final de l'API utilise explicitement `GOVERSION=go1.26.6` pour éviter que
le scanner lancé depuis la racine ne déclare la mauvaise version standard.

Il remonte **13 avis avec des symboles signalés comme appelés**, tous dans la
bibliothèque standard. Douze demandent au moins Go 1.26.9 et l'avis de certificats
[GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809) demande **1.26.10** sur cette
branche. Deux avis supplémentaires ne présentent pas de symbole appelé dans ce
scan : `GO-2026-6604` et l'avis OpenPGP `GO-2026-5932` du module `x/crypto`.

Action prioritaire : passer les chaînes de compilation et les contraintes Go à
1.26.10 ou à une version ultérieure corrigée et compatible, puis relancer les tests,
scanner les cibles Linux/Windows et reconstruire les exécutables. Penser aux huit
modules Go, au workflow macOS et à `scripts/setup-wsl-go.sh`.

L'analyse statique des appels ne prouve pas que chaque avis est exploitable par un
client distant dans la configuration déployée. Elle justifie la mise à jour ; elle
ne mesure pas l'exposition du serveur actif. Voir la
[méthode officielle de govulncheck](https://go.dev/doc/security/vuln/).

### Rust

Base RustSec utilisée : commit `640bf5a9f06e66d8721e8772766531fe77c8630d`,
mise à jour le 22 octobre. Résultats : **10 avis de vulnérabilité dans le verrouillage
serveur, 15 dans celui du client**. Certains sont communs aux deux.

| Bibliothèque | Serveur | Client | Minimum corrigé indiqué par les avis |
| --- | --- | --- | --- |
| quinn-proto | 0.11.17 | 0.11.15 | 0.11.18 |
| rustls | 0.23.45 | 0.23.45 | 0.23.46 |
| rustls-webpki | 0.103.15 | 0.103.15 | 0.103.16 sur la branche 0.103 |
| webpki-roots | 1.0.9 | 1.0.9 | 1.0.10 |
| webpki-root-certs | 1.0.9 | 1.0.3 | 1.0.10 |
| zip | — | 0.6.6 | 9.0.0, changement majeur à étudier |

Les alertes couvrent notamment des dénis de service QUIC, des comportements TLS
et la confiance dans les certificats. Par exemple,
[RUSTSEC-2026-0349](https://rustsec.org/advisories/RUSTSEC-2026-0349.html)
concerne une réduction de protection post-quantique sous certaines conditions,
pas un contournement général de l'authentification TLS.

Les trois avis `zip` portent sur des fonctions d'extraction. Le chemin lu dans
`rustdesk/src/plugin/manager.rs` utilise une extraction manuelle avec
`enclosed_name`, pas `ZipArchive::extract`. L'avis
[RUSTSEC-2026-0368](https://rustsec.org/advisories/RUSTSEC-2026-0368.html)
ne suffit donc pas à démontrer une exploitation de ce chemin. Vérifier les autres
usages et les dépendances transitives avant une migration majeure.

S'ajoutent 4 avertissements de non-maintenance côté serveur et 24 côté client,
ainsi qu'une version retirée du registre dans chaque verrouillage. Ces avertissements
ne sont pas comptés comme des exploitations démontrées. Les bibliothèques natives,
les DLL embarquées et les binaires distribués n'ont pas été rescannés exhaustivement.

Préparer les mises à jour ciblées dans les forks, compiler les moteurs pour les
plateformes distribuées, tester les connexions et la reconnexion, puis renouveler
les artefacts signés. Modifier les verrouillages seuls ne corrige aucun programme
déjà installé.

## Autres améliorations proposées, non appliquées

| Priorité | Constat et emplacement | Suite utile |
| --- | --- | --- |
| P2 | La CSP de `relaisdesk/client/index.html:10` autorise les scripts inline. Les fichiers du portail client ne définissent pas de protection `frame-ancestors` par en-tête ; les éventuels en-têtes OVH n'ont pas été inspectés. | Retirer progressivement les gestionnaires inline, puis durcir la CSP et vérifier la protection contre l'intégration dans une iframe, avec recette de la connexion Google et du lien vers le programme local. |
| P2 | `.github/workflows/build-macos.yml:61` et `:72` tolèrent les échecs du moteur et des tests. La présence de deux DMG différents ne valide pas leur contenu. | Rendre les étapes nécessaires bloquantes et contrôler le moteur embarqué, les architectures et les signatures lors d'une recette macOS. |
| P2 | `database/sessions.go:198` écrit `last_used_at` à chaque validation, avec jusqu'à cinq tentatives. | Mesurer les écritures et les temps d'attente SQLite ; envisager une actualisation espacée, en préservant le délai d'inactivité et la révocation. |
| P2 | `api/handlers/public_status.go` relâche son verrou avant les sondes : plusieurs appels sur cache expiré peuvent refaire simultanément les mêmes sondes. Les sondes sont séquentielles. | Mutualiser un rafraîchissement en cours, borner l'attente et paralléliser les sondes indépendantes ; tester cache expiré, panne et annulation. |
| P3 | `database/teams.go:75` effectue des requêtes supplémentaires par licence. | Mesurer sur des comptes comportant beaucoup de licences, puis regrouper les lectures et examiner les plans SQL avant d'ajouter des index. |
| P3 | `scripts/nginx-api-relaisdesk.conf.example:35` désactive la vérification TLS de l'amont, qui est sur la boucle locale. | Vérifier la chaîne du certificat côté Oracle et activer la validation avec le magasin de confiance adapté. L'exemple local ne prouve pas la configuration du serveur actif. |

Les gains de performance de ces propositions restent à mesurer. Aucune accélération
de la vidéo, du CPU ou du débit réseau n'est revendiquée par cet audit.

## Protections existantes vérifiées dans le périmètre

- Sessions opaques hachées, validation des licences actives, MFA et tests de révocation.
- Preuves signées des postes, contrôles de rejeu et restrictions de dossiers côté réseau.
- Manifeste de mise à jour signé et contrôle des empreintes des programmes.
- Sauvegardes SQLite cohérentes, chiffrement et restauration de contrôle.
- Durcissement systemd et conteneurs dans les configurations fournies.
- Pagination des listes du parc et cache du statut public.

Le contrôle ciblé des fichiers suivis du dépôt principal n'a trouvé que deux
fichiers de scripts contenant les marqueurs textuels recherchés de clés privées,
comme données de reconnaissance/formatage. Aucun secret réel n'a été affiché.
Ce contrôle ne couvre pas tout l'historique Git ni toutes les formes de secrets.

## Vérifications

- `go -C database test ./...` : réussi sous Windows.
- API complète `go -C api test ./...` : réussie sous Linux/WSL, y compris sauvegarde,
  paiements simulés, authentification et autorisations.
- Sous Windows, les tests API passent hors de l'assertion Unix `0600` du manifeste
  de sauvegarde ; cette assertion est validée sous Linux, sans la retirer du test.
- `go -C tests test ./...` : réussi après correction des fixtures historiques.
- Modules keygen, configurateur, viewer, debpack et dashboard_admin : réussis sous
  Windows avec `-tags ci`. Les interfaces graphiques natives ne sont pas validées
  par ce mode de test.
- `go vet` sur API et database : réussi.
- `go -C api test -race ./middleware` sous Linux : réussi, aucune course de données
  signalée dans les tests exécutés.
- 91 tests Python des scripts : réussis.
- Contrôles JavaScript Google/MFA, équipes et parc : réussis.
- Syntaxe et activation des deux service workers : vérifiées.
- Les tests utilisant le réseau local ou les permissions système ont été relancés
  hors des restrictions du bac à sable lorsqu'elles empêchaient leur exécution.
- Les moteurs Rust et les interfaces Flutter/macOS n'ont pas été recompilés pendant
  cette revue. Les scanners de dépendances ne remplacent pas cette recette.

## Fichiers et mise en service

Pour OVH, les seuls fichiers web modifiés sont :

- `relaisdesk/admin/sw.js` ;
- `relaisdesk/technicien/sw.js`.

Les transférer à leurs emplacements habituels. Aucune archive ZIP ni copie de
livraison web n'a été créée.

Les changements de sécurité de l'API sont dans `api/middleware/` et nécessitent
une reconstruction puis un déploiement Oracle distinct. Les nouvelles protections
ne sont donc pas encore actives en production du seul fait de cette modification
locale. Les tests sont dans ce même répertoire et dans
`tests/admin_dashboard_test.go`. Les dépendances signalées restent à mettre à jour
avant de qualifier une nouvelle livraison complète comme corrigée.
