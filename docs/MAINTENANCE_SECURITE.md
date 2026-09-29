# Maintenance de sécurité à la demande

Ce script prépare des mises à jour de dépendances dans **`update/`**, à la racine du projet. Il n'est pas un service permanent : il ne fonctionne que lorsque vous le lancez.

Par défaut, il ne modifie pas les sources originales, ne contacte pas Oracle en SSH, ne déploie rien sur OVH/GitHub et ne publie aucun binaire. Les changements préparés restent à vérifier et à tester avant intégration. L'option explicite `-ApplyFixes` peut intégrer les seuls fichiers de dépendances autorisés, après audit et tests réussis, vérification des empreintes et sauvegarde locale.

## 1. Le lancer sur Windows

Ouvrir PowerShell **sans demander les droits administrateur**, dans le dossier du projet, puis exécuter :

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\update-security.ps1
```

`-ExecutionPolicy Bypass` s'applique uniquement au processus PowerShell lancé ici : aucune politique Windows persistante n'est changée. Il permet de lancer ce script local non signé lorsque PowerShell bloque les scripts par défaut. Si une politique de votre organisation l'interdit, ne pas la contourner ; demander son accord.

Si l'exécution de scripts est déjà autorisée, la commande courte suffit :

```powershell
.\scripts\update-security.ps1
```

Laisser la fenêtre ouverte jusqu'à l'affichage du chemin de `rapport.md`. Selon les téléchargements et les outils disponibles, cela peut prendre plusieurs minutes. Un message d'avancement est affiché pour les commandes longues. La limite est de 15 minutes **par commande**, pas pour l'ensemble du traitement.

Chaque lancement repart des sources présentes à la racine, **pas d'un ancien dossier `update`**. Les modifications de travail non encore commitées sont copiées telles quelles ; il est préférable de ne pas modifier le projet pendant l'opération.

## 2. Où sont les résultats ?

```text
update/
  AAAAMMJJ-HHMMSS-identifiant/
    rapport.md                 résumé lisible, alertes restantes, limites
    rapport.json               résultats détaillés et états des contrôles
    projet/                    copie de travail des composants concernés
    fichiers-corriges/          uniquement les fichiers de dépendances modifiés
    modifications.patch        différences avant/après
    inventaire-initial.json    empreintes SHA-256 des sources copiées
    exclusions.json            fichiers écartés (sans leur contenu)
    audits/                    résultats d'audit structurés avant/après
    logs/                      sortie de chaque commande
    tentatives-rejetees/        modifications abandonnées après un échec, si besoin
    tools/                     outils installés si l'option correspondante est choisie
```

Les anciens passages sont conservés, sans écrasement ni nettoyage automatique. Prévoir de l'espace disque. `update/` est exclu du suivi Git à la racine.

`fichiers-corriges/` contient des **candidats de mise à jour**, et non un produit certifié prêt à déployer. Si un test échoue ou un contrôle reste incomplet, le rapport indique **NE PAS appliquer ce lot en l'état**. Les modifications du composant qui a échoué peuvent encore être présentes pour analyse ; ne pas les intégrer aveuglément.

La copie `projet/` n'est ni une sauvegarde complète ni une archive de publication des sources : secrets, `.git`, téléchargements, bases de données et une partie des sorties de compilation sont exclus. Les quatre moteurs déjà embarqués dans `installer/*/embedded/rustdesk.exe` et `.deb` sont conservés **inchangés comme entrées de compilation** ; ils ne sont pas exportés dans `fichiers-corriges/`.

## 3. Ce qui est automatisé

| Partie | Audit | Mises à jour tentées |
| --- | --- | --- |
| 8 modules Go : API, base, tests, keygen, configurateur, viewer, debpack, tableau de bord | `govulncheck` au niveau module | Dépendances avec `go get …@upgrade`, puis correctif de la version Go ; conservation des `replace`, contrôle `go mod verify` |
| Client Rust, serveur Rust, ancienne interface Rust serveur | `cargo-audit`, avec actualisation de la base RustSec | Mises à jour ciblées des crates de registre signalées, dans les contraintes existantes ; aucune montée de révision Git automatique |
| Ancienne interface web serveur | `npm audit` | `npm audit fix --package-lock-only --ignore-scripts`, **sans `--force`** et sans changement de `package.json` |
| Packages Flutter hébergés sur Pub | OSV à partir de `pubspec.lock` | Mise à jour ciblée des packages signalés, si un SDK Flutter approprié est disponible ; conservation des blocs Git et des contraintes |
| Générateur portable Python | OSV pour Brotli | Épinglage sur la version stable publiée par PyPI, si elle n'est pas signalée par OSV ; pas d'installation Python globale |

Les mises à jour Go peuvent changer les exigences de compilation, et une version mineure peut encore introduire une régression. « Résolu par le gestionnaire de dépendances » ne signifie pas « application validée ».

Les correctifs locaux atty, GLib et PHF sont vérifiés avec les contrôles d'empreintes déjà présents dans les forks. Si une opération échoue ou déplace une révision Git Rust/Dart, ses fichiers de dépendances sont restaurés **dans la copie**, et la tentative est conservée pour examen.

Les contrôles existants des tarifs et de cohérence des documents contractuels sont également exécutés sur la copie. Ce sont des tests de non-régression, pas un nouvel avis juridique.

Les scanners consultent des bases publiques et peuvent transmettre aux registres les noms/versions des dépendances. Le script n'envoie pas les sources du projet à un service d'IA.

## 4. Prérequis et options

Python **3.11 ou plus récent** est nécessaire ; aucune bibliothèque Python tierce n'est requise pour ce script. Le lanceur recherche Python installé, `py`, puis le Python fourni avec l'environnement local de travail s'il existe.

Pour les différents composants : Go, Rust/Cargo et Git, Node.js/npm, `govulncheck`, `cargo-audit`. Les deux scanners sont aussi recherchés dans les emplacements `.tools/` déjà utilisés par ce projet. Leur absence est signalée, jamais assimilée à « aucune faille ».

Pour actualiser/installer les deux scanners dans le dossier de ce passage :

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\update-security.ps1 -InstallAuditTools
```

Cette option télécharge et compile des outils tiers ; Cargo/Go et les outils de compilation doivent déjà fonctionner. Elle peut être longue. Elle n'installe pas automatiquement Python, Node, Go, Rust, Flutter ou Visual Studio. Les scanners ainsi installés ne sont pas repris automatiquement d'un ancien passage ; utiliser cette option au besoin ou maintenir une installation locale des scanners.

Autres modes :

```powershell
# Audit uniquement : pas de mise à jour des dépendances
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\update-security.ps1 -AuditOnly

# Vérifier seulement la préparation de la copie, sans réseau ni outil externe
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\update-security.ps1 -PrepareOnly

# Préparer les mises à jour et ajouter les tests/compilations ciblés
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\update-security.ps1 -RunBuildTests
```

`-RunBuildTests` ajoute les tests Go et l'audit des symboles utilisés, les tests atty, les tests d'autorisation serveur, un `cargo check` de l'ancienne interface et la construction Vite. Des compilateurs natifs, SDK et bibliothèques adaptés peuvent manquer : le rapport le dira. Il ne lance **pas** toute la suite de tests du client RustDesk : certains tests système amont ont des effets sur la machine. Il ne reconstruit pas les installateurs de livraison ni toutes les plateformes Flutter.

Les tests/compilations exécutent du code du projet et de dépendances : une copie de fichiers **n'est pas un bac à sable système**. Utiliser un compte non administrateur et, pour une isolation plus forte, une VM dédiée. Les caches habituels de Go, Cargo, npm et Dart peuvent être alimentés dans votre profil utilisateur.

Les options `-PythonPath "C:\chemin\python.exe"` et `-CommandTimeoutSeconds 1800` permettent de choisir Python ou d'augmenter le délai maximal d'une commande. `-AuditOnly` et `-PrepareOnly` sont incompatibles.

Codes de sortie du lanceur :

- `0` : préparation terminée, ou contrôles automatiques terminés sans alerte finale ; la recette reste nécessaire.
- `2` : alertes restantes, contrôle incomplet, échec ou interruption. Lire le rapport et les journaux.
- `1` : erreur de préparation empêchant le démarrage normal.

Une connexion bloquée, une réponse invalide ou incomplète de la base de vulnérabilités, ou un outil absent entraîne un état **INCONNU / ERREUR**, pas un résultat rassurant artificiel.

## 5. Ce qui ne peut pas être corrigé automatiquement

Il faut distinguer **« pas corrigible par ce script »** et **« impossible à corriger »** : plusieurs cas nécessitent une migration ou un développement manuel.

- **Failles sans version corrigée compatible** : chaque identifiant CVE/GHSA/RUSTSEC/GO détecté et encore présent est listé dans `rapport.md`, avec les versions corrigées indiquées par la source, lorsqu'elles existent. Une version corrigée peut se trouver hors des contraintes actuelles et demander un portage.
- **Bibliothèques non maintenues**, notamment GTK3 et sodiumoxide déjà identifiées dans le projet : changer seulement un numéro de version ne supprime pas cette dette. Une alerte de non-maintenance n'est pas, à elle seule, la preuve d'une faille exploitable.
- **Composants natifs et historiques** : codecs, vcpkg, Sciter, pilotes, SDK, CocoaPods et Gradle demandent une vérification distincte et souvent une compilation adaptée à chaque plateforme. Ils ne sont pas automatiquement actualisés ici.
- **Dépendances Git et correctifs propres aux forks** : un changement de commit ou un remplacement de backport exige une revue de nos modifications. Le script les préserve.
- **Bugs de notre code** : contrôles d'accès, licences simultanées, validation des entrées, races, logique métier, protocoles réseau. Un scanner de dépendances ne démontre pas leur sûreté.
- **Infrastructure et secrets** : mises à jour Ubuntu/Docker/Nginx, pare-feu, paramètres Oracle, certificats et rotation des clés sont hors périmètre. Les exclusions évitent les secrets connus ; elles ne garantissent pas de découvrir un secret inscrit dans un fichier source inhabituel. Ne pas publier le dossier `update` entier.
- **Failles encore inconnues ou absentes des bases publiques** : aucun outil ne garantit zéro faille.
- **Binaires existants** : ils restent inchangés. Corriger les dépendances dans les sources ne répare pas les exécutables déjà construits ou installés.

Les alertes Go au niveau module peuvent concerner un paquet non importé. L'option d'audit des symboles aide à préciser l'exposition pour la plateforme courante ; elle ne couvre pas toutes les plateformes et tous les scénarios d'exécution.

### Premier audit de validation — 8 septembre 2026

Le [rapport de validation en lecture seule](../update/20260908-205353-a8ce1b1e/rapport.md) contient les identifiants détaillés. À cette date :

- Les trois audits Rust ne trouvent pas de vulnérabilité dans la catégorie `vulnerabilities` de RustSec, mais conservent **26 avertissements de non-maintenance** au total (certaines dépendances sont présentes dans plusieurs composants), ainsi que deux versions retirées du registre : `spin 0.9.8` et `chacha20 0.10.1`. Un retrait n'est pas à lui seul une preuve de faille ; le mode mise à jour essaiera une version compatible.
- Les avertissements de non-maintenance incluent notamment **sodiumoxide — RUSTSEC-2021-0137**, **bincode — RUSTSEC-2025-0141**, **ansi_term — RUSTSEC-2021-0139** et **dlopen_derive — RUSTSEC-2023-0051**. Le remplacement de ces composants peut nécessiter une migration manuelle, pas seulement un nouveau verrou de dépendances.
- **GO-2026-5932** apparaît au niveau module dans l'API, la base et keygen. L'avis concerne `golang.org/x/crypto/openpgp`, déclaré non maintenu et dangereux par conception. Ce n'est pas la preuve que ce paquet est utilisé par ces programmes : ne pas assimiler la présence du module `x/crypto` à une exploitation démontrée. Aucune version corrigée n'est indiquée dans cet avis ; si OpenPGP est effectivement utilisé, une migration est à analyser.
- Les audits npm et des packages Pub hébergés ne remontent pas d'alerte. Cela ne couvre pas les composants natifs ni les dépendances Dart Git/path/SDK.
- Le fichier Python ne fixe pas la version de Brotli : l'audit signale donc une version historique **indéterminable**. Le mode mise à jour prépare son épinglage, après consultation de PyPI et d'OSV.

Ce passage de validation n'a effectué **aucune mise à jour de dépendance** : ses dossiers `fichiers-corriges/` et son patch ne contiennent donc pas de correction à appliquer. Les alertes futures peuvent différer.

## 6. Après une exécution

1. Lire `rapport.md`, puis les journaux associés aux erreurs ; ne pas appliquer un lot marqué à vérifier sans analyse.
2. Examiner `modifications.patch` et les empreintes, puis tester les candidats dans la copie ou dans un environnement de compilation contrôlé. L'intégration n'est jamais implicite ; voir la section 8 pour l'application protégée.
3. Recompiler les composants concernés. Si Go a été actualisé, employer le compilateur requis, y compris pour la cible Linux/Oracle.
4. Faire la recette : connexion directe et relayée, autorisation obligatoire, licences simultanées, reconnexion, transferts, launchers/viewer et parcours commerciaux concernés.
5. Après validation humaine, intégrer les changements appropriés aux dépôts, préparer les sources correspondant exactement aux binaires, actualiser les empreintes et signer le manifeste de livraison selon le processus du projet.
6. Déployer uniquement après votre décision explicite. **Ce script ne publie jamais les nouveaux binaires à votre place.**

## 7. Vérification du script lui-même

Tests hors réseau :

```powershell
python -B .\scripts\test_security_maintenance.py
python -B .\scripts\test_maintenance_apply.py
python -B .\scripts\test_update_scripts.py
node .\scripts\test_verify_security_release.mjs
```

Ils couvrent exclusions, non-écrasement des sources, export limité, restauration sur échec, conservation des révisions Git, erreur réseau/outils manquants, arrêt sur délai, interruption et formats des rapports. Le test de lien symbolique est ignoré si Windows ne permet pas d'en créer. Ils ne garantissent pas le comportement de versions futures des gestionnaires de dépendances : les contrôles avant/après servent également à détecter leurs changements.

Validation du 8 septembre 2026 : **31 tests réussis, 1 ignoré** (création de liens symboliques non autorisée sur le compte de test), lanceur PowerShell testé en préparation puis audit réel, contrôles locaux des backports et tests de cohérence tarifs/documents réussis. Le mode de modification est couvert par des tests simulant les outils ; une mise à jour réseau complète et les compilations facultatives n'ont pas été exécutées lors de cette livraison du script.

Références des comportements utilisés : [Cargo update](https://doc.rust-lang.org/cargo/commands/cargo-update.html), [gestion des dépendances Go](https://go.dev/doc/modules/managing-dependencies), [govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck), [npm audit](https://docs.npmjs.com/cli/v11/commands/npm-audit/), [Dart pub upgrade](https://dart.dev/tools/pub/cmd/pub-upgrade), [OSV querybatch](https://google.github.io/osv.dev/post-v1-querybatch/).

## 8. Application et restauration protégées — 19 septembre 2026

```powershell
# Audit, corrections et tests dans update/, sans toucher aux sources originales
.\scripts\update-security.ps1 -RunBuildTests -ShowReport

# Nouvelle maintenance avec application explicite aux sources si tous les contrôles passent
# (confirmation interactive demandée avant toute écriture)
.\scripts\update-security.ps1 -ApplyFixes -ShowReport

# Simuler cette application sans rien modifier
.\scripts\update-security.ps1 -ApplyFixes -DryRun -ShowReport

# Ou intégrer précisément un lot déjà testé et examiné (remplacer IDENTIFIANT)
python -B .\scripts\maintenance_apply.py --project-root . --run-id IDENTIFIANT

# Annuler précisément CE lot de dépendances, avec confirmation (aucun audit relancé)
.\scripts\update-security.ps1 -Rollback -RunId IDENTIFIANT -ShowReport

# Afficher le plan puis restaurer via l'orchestrateur (aucun audit relancé, confirmation demandée)
.\scripts\update-and-rebuild-programs.ps1 -Rollback -RunId IDENTIFIANT -DryRun
.\scripts\update-and-rebuild-programs.ps1 -Rollback -RunId IDENTIFIANT
```

`IDENTIFIANT` est le nom exact du dossier horodaté créé sous `update/`. Aucun script ne sélectionne « le plus récent ». L'application vérifie le rapport, les tests demandés, les alertes, la liste blanche, les empreintes avant/après et l'absence de liens/jonctions. Elle ne copie ni le fichier d'information ni des fichiers ajoutés hors rapport.

Les originaux et le journal sont conservés dans `update/IDENTIFIANT/application/`. Une erreur intermédiaire déclenche une restauration des seuls fichiers déjà remplacés, à condition qu'ils n'aient pas été modifiés depuis. Le retour arrière explicite refuse également les modifications ultérieures de l'utilisateur. Il ne touche jamais aux exécutables, au site ni à Oracle.

Un verrou `update/.apply.lock` interdit deux applications/restaurations simultanées. Après un arrêt brutal, **ne pas supprimer ce verrou à l'aveugle** : vérifier qu'aucun processus ne travaille encore, examiner `application/receipt.json` et comparer les empreintes aux sauvegardes. Un état `applying`, `rollback_in_progress` ou `recovery_required` demande une récupération contrôlée ; il n'est pas déclaré réussi. Ne pas éditer les fichiers pendant l'application : le verrou coordonne les scripts, pas les autres éditeurs.

`-ApplyFixes` impose les tests et refuse `-AuditOnly`/`-PrepareOnly`. Un avertissement non résolu ou un outil manquant interdit l'application automatique : cela peut nécessiter une analyse manuelle, pas un contournement du contrôle. Depuis le lanceur, toute écriture (`-ApplyFixes`, `-Rollback`) demande une confirmation interactive, sauf `-DryRun` (simulation sans écriture) ou `-Force` (usage non interactif, à réserver aux appels déjà autorisés explicitement, comme l'orchestrateur avec `-ApplySecurityFixes`). `-Rollback` exige le `-RunId` du lot et n'exécute aucun audit. L'option `-Rollback` de l'orchestrateur appelle le lanceur : même confirmation, avec `-Force` transmis quand il est présent.

## 9. Script tout-en-un : compiler un candidat sans publication

Le menu de `update-and-rebuild-programs.ps1` conserve la maintenance locale, la simulation et l'aide à la restauration. **Il ne déploie plus sur Oracle, ne restaure plus les anciennes sauvegardes partielles de téléchargements et ne régénère plus les clés.** Les archives déjà existantes restent intactes.

Pour compiler, renseigner explicitement une nouvelle version, la clé de signature existante et un manifeste d'entrées basé sur `scripts/maintenance-engines.example.json`. Remplacer chaque chemin et chaque empreinte par ceux des moteurs réellement compilés et approuvés. Les chemins relatifs sont résolus depuis le dossier du manifeste. Ne jamais mettre la clé privée dans ce manifeste.

```powershell
.\scripts\update-and-rebuild-programs.ps1 `
  -Version '1.0.5' `
  -EngineManifest 'C:\chemin\moteurs-valides.json' `
  -ReleaseSigningKeyPath 'C:\chemin-prive\release-signing-ed25519' `
  -ReleasePublicKey 'CLE_PUBLIQUE_EXISTANTE_BASE64URL' `
  -ShowReport
```

La version ci-dessus est un **exemple**, à choisir selon les versions réellement livrées. Le chemin de clé n'est plus remplacé silencieusement par une autre clé. Le générateur du manifeste signé vérifie que la clé publique correspond à la clé privée ; cela ne prouve pas qu'elle est celle attendue par les clients installés, qu'il faut conserver.

Par défaut, le moteur Windows est reconstruit **dans la copie auditée**, avec la chaîne native déjà préparée sous `.tools/`. Aucun téléchargement implicite d'une chaîne native complète n'est fait par l'orchestrateur. Le reçu exact de cette construction fournit le moteur natif et ses DLL ; aucune sélection par date dans les caches.

`-SkipEngineBuild` réutilise explicitement les quatre entrées Windows du manifeste (portable, service natif, Sciter et affichage virtuel). Les trois entrées Linux sont toujours nécessaires : paquet `.deb`, exécutable RustDesk et bibliothèque `librustdesk.so`. Leurs empreintes sont vérifiées et les fichiers sont copiés dans le lot. Les empreintes Cargo du manifeste doivent correspondre aux sources auditées : si les dépendances natives changent, recompiler les moteurs concernés puis revoir les entrées, sans simplement recopier une ancienne empreinte. Une déclaration d'empreintes est un contrôle d'intégrité, **pas une preuve indépendante de provenance**.

Les lanceurs sont compilés depuis les sources du même lot, avec version et empreintes Windows/Linux injectées, dépendances Go en lecture seule, tests Windows et compilation des tests Linux. Les tests Linux doivent ensuite être exécutés sur Linux. `-SkipNSIS` produit uniquement les portables Windows et les paquets Linux. Le lot signé est revérifié indépendamment (signature, noms, tailles, empreintes et somme de contrôle) avant d'être annoncé prêt pour la recette.

Résultats : `update/IDENTIFIANT/release-candidate/downloads/` et `build-result.json`. **Rien n'est copié dans `relaisdesk/downloads/` ni envoyé au serveur.** Un build échoué reste un dossier de diagnostic, sans `build-result.json` de réussite. `-SkipSecurityAudit` est réservé à une compilation explicitement non auditée : avertissement et marqueur persistant `audit_skipped=true`, jamais une validation de sécurité.

## 10. Déploiement et clés : opérations séparées

Les anciennes fonctions génériques de production du menu ont été retirées, car une livraison peut comporter des migrations de base et des contraintes de compatibilité que ce menu ne vérifiait pas. Les scripts datés de déploiement restent des **procédures historiques spécifiques**, à ne pas relancer aveuglément pour une nouvelle version.

Avant une livraison Oracle : recette des programmes, sauvegarde SQLite cohérente et vérifiée, test de migration sur une copie, contrôle des empreintes et de l'identité SSH du serveur, verrou de déploiement, préparation hors des fichiers servis, bascule contrôlée, contrôle TLS/HTTP et contenu de santé, vérification du binaire actif et plan de retour arrière compatible avec la base. Pour les téléchargements : publier uniquement un lot complet signé et validé, sans mélanger ancien manifeste et nouveaux fichiers. La préparation locale n'autorise pas cette livraison. VPN et mises à jour système restent hors périmètre.

```powershell
python -B .\scripts\manage-secrets.py --action plan
```

Cette commande explique les conditions de chaque rotation ; `show` donne seulement la présence de quelques fichiers **locaux**, sans lire leur contenu. L'ancienne action `regenerate`, y compris `--target all`, échoue avant toute opération. Aucune copie supplémentaire des secrets n'est créée dans `backup/`.

- **Clé HMAC des essais** : liée à l'historique anti-abus ; ne pas changer sans migration dédiée. Le verrou de la base serait sinon déclenché.
- **Signature des versions** : transition de confiance des clients déjà installés et du manifeste API indispensable avant bascule.
- **Autorisation réseau** : coordonner les clés de hbbs/hbbr, l'identifiant du signataire API et la durée de validité des jetons.
- **Chiffrement des sauvegardes** : conserver les clés anciennes hors serveur avec leur correspondance aux archives et tester le déchiffrement.
- **Jeton administrateur** : rotation distincte, sauvegarde protégée, reconfiguration des utilisateurs du jeton et validation après changement.

Une mise à jour de dépendances ne justifie pas à elle seule une rotation de ces clés.

### Validation de cette révision

Le 19 septembre 2026 : 34 tests du moteur de maintenance réussis et 1 ignoré (création de liens symboliques non autorisée), 19 tests d'application/restauration et de politique des clés réussis, 13 tests d'intégration PowerShell réussis, et 7 contrôles de lots signés réussis. Les builds/audits des tests d'intégration sont simulés dans des dossiers temporaires ; le mode préparation a aussi été exercé avec le vrai moteur Python dans un projet factice. Les quatre scripts PowerShell modifiés passent l'analyse syntaxique.

Aucune compilation complète des programmes, mise à jour réseau des dépendances, livraison Oracle ou rotation de clé n'a été exécutée pour cette révision. Les tests démontrent les garde-fous couverts, pas l'absence de toute faille ni la validité d'une future livraison.
