# Correctifs complémentaires — 23 octobre 2026

Suite de l'[audit du 22 octobre](AUDIT_SECURITE_PERFORMANCES_2026-10-22.md), à la
demande de terminer les travaux en restant dans la version produit **1.0**.
Les valeurs par défaut `APP_VERSION = "1.0.0"` des programmes ne sont pas modifiées.
Les versions techniques Go/Rust et des bibliothèques évoluent indépendamment.

## Modifications

### Dépendances

- Les huit modules Go exigent maintenant Go **1.26.10**, comme le workflow macOS
  et le script de préparation WSL. Cette version corrige les avis de la bibliothèque
  standard identifiés dans l'audit.
- Les interfaces Viewer, Technicien et dashboard utilisent `golang.org/x/net`
  **0.60.0**, avec les versions `x/sys` et `x/text` compatibles résolues par Go.
- Les deux verrouillages Rust utilisent `quinn-proto 0.11.18`, `rustls 0.23.46`,
  `rustls-webpki 0.103.16`, `webpki-roots 1.0.10` et `webpki-root-certs 1.0.10`.
- Le client utilise maintenant `zip 9.0.5`. Le serveur abandonne la version retirée
  `chacha20 0.10.1` pour `0.10.2`.
- Le scan RustSec final des deux verrouillages indique **zéro avis de vulnérabilité**
  dans la base consultée. Les avertissements de non-maintenance restent visibles ;
  le remplacement des bibliothèques historiques n'est pas assimilé à une correction
  de faille démontrée.

Les versions des paquets RustDesk eux-mêmes restent celles des forks. Les changements
de dépendances et de code sont locaux, dans leurs dépôts respectifs ; ils devront
être publiés avec les sources correspondantes lors d'une livraison des moteurs.

### SQLite et sessions

`last_used_at` est persisté au plus une fois toutes les 30 secondes par session.
La première lecture évite la tentative d'écriture quand l'activité est récente,
et une condition SQL empêche plusieurs requêtes simultanées d'écrire la même
actualisation. Aucune identité ni autorisation n'est mise en cache : expiration,
licence active et révocation continuent d'être vérifiées à chaque requête.

La limite d'inactivité n'est pas prolongée. Sa précision de persistance est de
30 secondes : une session peut expirer jusqu'à 30 secondes plus tôt que si chaque
requête avait été enregistrée individuellement. Les difficultés transitoires
d'écriture ne transforment toujours pas une session valide en refus d'authentification.

Le test avec un déclencheur SQLite confirme : aucune écriture pour vingt validations
récentes, une seule actualisation pour vingt validations simultanées d'une activité
ancienne, et refus immédiat après suppression de la session.

### Lecture des licences d'équipe

La liste utilise une lecture groupée au lieu d'une boucle de lectures par licence.
La règle de calcul d'éligibilité est partagée avec les lectures individuelles.
Deux index couvrent la recherche des membres par licence/statut et celle des essais
par licence. L'ordre, l'isolation entre propriétaires et la prise en compte de
l'expiration des invitations sont conservés.

Mesure locale sur cinquante licences, même processus et même base de test :

| Parcours | Temps moyen observé |
| --- | ---: |
| Lectures individuelles des cinquante licences | 31,77 ms |
| Lecture groupée | 1,72 ms |

Commande : `go -C database test -run '^$' -bench BenchmarkTeamLicenseListing -benchtime=300ms`.
Le parcours individuel du benchmark n'inclut même pas la requête initiale qui
récupérait les identifiants dans l'ancien code. Cette mesure sur le poste de
développement ne constitue pas un engagement de temps de réponse en production.

### Supervision

Un cache expiré déclenche un seul rafraîchissement partagé. Les sondes indépendantes
de SQLite, hbbs, hbbr et SMTP sont lancées en parallèle, avec un délai global
de cinq secondes. L'annulation d'une requête libère son appelant et n'annule pas
le rafraîchissement dont les autres clients ont besoin.

Un test force les quatre sondes à attendre ensemble, annule le premier appelant,
puis sert trente autres requêtes avec ces quatre sondes seulement.

### Portail client et OVH

- Suppression de `unsafe-inline` dans la directive `script-src` du portail.
- Suppression du gestionnaire inline de chargement Google ; l'initialisation
  existante dans le fichier JavaScript externe assure le chargement du bouton.
- Ajout d'un en-tête CSP `frame-ancestors 'none'`, de `X-Frame-Options: DENY`, de
  `nosniff`, de la politique sans référent et des directives de non-indexation
  dans `relaisdesk/client/.htaccess`.

La directive de style `unsafe-inline` est conservée pour les styles existants et
le bouton Google. La politique CSP du document et l'en-tête de protection contre
les iframes s'appliquent conjointement.

Fichiers à transférer sur OVH pour ce complément :

- `relaisdesk/client/index.html` ;
- **`relaisdesk/client/.htaccess`**, fichier caché à inclure dans le transfert.

Pour l'ensemble des correctifs depuis le début de l'audit, transférer aussi
`relaisdesk/admin/sw.js` et `relaisdesk/technicien/sw.js`.
Le dossier `relaisdesk/` demeure la référence ; aucun ZIP ni copie de livraison web
n'a été créé.

### Configuration Oracle

L'exemple Nginx active la vérification TLS du serveur API local et utilise le
magasin système `/etc/ssl/certs/ca-certificates.crt`. Le nom vérifié et le SNI
restent `api.relaisdesk.fr`. Cette configuration doit être validée sur Oracle avec
le certificat réellement servi avant son activation ; l'exemple seul ne prouve
pas l'état de la chaîne du serveur actif.

### Compilation macOS

Les échecs du moteur, des tests et de la création des DMG deviennent bloquants.
Chaque architecture utilise un runner natif : `macos-15` pour ARM et
`macos-15-intel` pour Intel, selon la
[référence officielle des runners](https://docs.github.com/en/actions/reference/runners/github-hosted-runners).
Les deux architectures exécutent les tests Go.
Le clonage inclut les sous-modules. La préparation installe les dépendances natives
avec la révision vcpkg du fork, NASM 2.16.03 avec contrôle SHA-512, et génère le pont
Flutter/Rust avec les versions d'outils du fork. La construction active la fonction
`flutter`, place la bibliothèque Rust à l'emplacement attendu par Xcode et utilise
les cibles minimales macOS adaptées aux deux architectures. Les tests Go utilisent
le mode `ci`, tandis que les deux compilations finales construisent les interfaces
natives.

Le workflow vérifie la présence et l'architecture de l'exécutable RustDesk, injecte
son empreinte dans les lanceurs, embarque le moteur, ses Frameworks et ses ressources
dans les deux bundles, et vérifie les copies. Les fichiers Info.plist identifient
les lanceurs respectifs et portent la version produit `1.0.0`. Le moteur est
construit avec le verrouillage Cargo, et un artefact manquant bloque la livraison.
Le moteur et les bundles reçoivent une signature locale ad hoc, vérifiée avec
`codesign --verify --deep --strict` ; les empreintes du moteur sont contrôlées
après cette opération. Cette signature ne constitue pas une identité Developer ID
Apple ni une notarisation.

Le workflow dépend du fork distant : les changements locaux du client doivent
être publiés dans ce fork avant son utilisation pour une livraison. Un audit RustSec
bloquant empêche de livrer silencieusement un verrouillage distant encore vulnérable.
Le contrôle
syntaxique local ne remplace pas une exécution sur Mac, ni une signature Apple
et une notarisation lorsqu'elles sont nécessaires à la distribution.

## État des vérifications et de la livraison

Vérifications terminées :

- Base de données complète et tests d'intégration Go : réussis.
- API complète sous Linux/WSL avec Go 1.26.10 : réussie, y compris sauvegardes et
  paiements simulés. Binaire Linux de contrôle compilé dans
  `.cache/api-linux/api-followup`.
- Tests avec détection de courses de données sous Linux : supervision publique,
  middlewares, activité et validation concurrente des sessions, lecture des équipes :
  réussis. La suite fonctionnelle complète est exécutée sans instrumentation ;
  la détection de courses de données est ciblée sur les changements de concurrence.
- Modules keygen, configurateur, viewer, debpack et dashboard_admin : tests réussis
  sous Windows ; les interfaces sont testées avec `-tags ci`.
- `go vet` de l'API et de la base : réussi.
- Scanner Go : aucun avis dans les trois interfaces après mise à jour réseau ;
  l'API ne conserve que l'avis OpenPGP du module `x/crypto`, sans paquet ni symbole
  appelé dans le scan. Résultats bruts : `.cache/vuln-followup-api.json` et
  `.cache/vuln-final-*.json`.
- Scanner RustSec : zéro avis de vulnérabilité dans les deux verrouillages, avec
  la base au commit `640bf5a9f06e66d8721e8772766531fe77c8630d`, actualisée le
  22 octobre et reconsultée le 23 octobre. Les avertissements de maintenance sont
  conservés dans `.cache/audit-rust-*-followup.json`.
- Serveur Rust sous Windows : 15 tests réussis.
- `cargo check --locked --bins` du serveur : réussi pour hbbs, hbbr et l'utilitaire.
- Client Rust sous Windows : bibliothèque compilée et deux tests de sécurité ciblés
  réussis (preuve de possession, droits de session et refus des rejeux côté poste).
  Les cent autres tests du moteur ne sont pas annoncés comme exécutés par ce filtre.
- Scripts Python : 91 tests réussis.
- JavaScript : contrôles Google/MFA, parc et équipes réussis ; absence de
  gestionnaires inline et protection contre les iframes contrôlées dans les sources.
- Workflow macOS : `actionlint 1.7.12` réussi. Aucune exécution macOS n'est revendiquée.

- Client avec le module de plugins : `cargo check --locked --lib --features plugin_framework`
  réussi sous Windows, y compris l'usage de ZIP 9 dans le code d'installation.
- Le dernier contrôle `actionlint 1.7.12` après alignement de la préparation Flutter
  et des dépendances natives est réussi.
- Vérification finale des différences : aucune erreur d'espaces Git ; les valeurs
  applicatives restent `1.0.0` dans `installer/viewer/config.go` et
  `installer/configurator/updates.go`.

À la clôture de cette passe d'audit, aucune modification n'avait encore été
déployée. L'exploitant a ensuite autorisé le déploiement hors OVH. Son état et les
correctifs natifs supplémentaires sont suivis dans le
[compte rendu de déploiement](../rapports/DEPLOIEMENT_SECURITE_PERFORMANCES_2026-10-23.md).
Les programmes déjà installés ne sont pas corrigés par la modification des sources
seule.
