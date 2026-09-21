# Correctifs de sécurité — 19 septembre 2026

## Périmètre et état

Revue locale des évolutions récentes : authentification Google, double
authentification, sessions, droits des techniciens, agent de parc Linux/Windows,
mises à jour automatiques et dépendances Go/Rust.

Aucun déploiement sur Oracle, aucune publication GitHub, aucune modification des
exécutables de distribution ni du manifeste public. Le VPN et la configuration
du serveur n'ont pas été touchés. Les fichiers générés pendant les vérifications
sont des binaires de **test**, pas une livraison client.

Cette revue et les scanners ne constituent pas une garantie d'absence de toute
vulnérabilité. Les dépendances natives, tous les systèmes d'exploitation et une
session de contrôle à distance complète n'ont pas fait l'objet d'une recette exhaustive.

## Failles confirmées et corrections

### 1. Connexion Google : contournement de la double authentification

La connexion Google créait immédiatement une session client même lorsque le compte
avait activé la double authentification RelaisDesk. Un test de régression a
reproduit ce comportement avant correction.

La connexion crée maintenant un challenge limité à cinq minutes, sans session
utilisable avant validation du code TOTP ou d'un code de secours. Un code envoyé
à la même boîte Google ne peut pas servir de second facteur pour ce parcours.
Le parcours par mot de passe conserve son fonctionnement existant.

L'interface client traite cette réponse et ne stocke pas de faux jeton avant la
validation. L'ancienne fonction de création de session refuse les comptes MFA.

Une migration idempotente `20260919-google-mfa-v1` expire une seule fois les
sessions existantes des comptes avec MFA, ainsi que les sessions technicien
associées et appareils de confiance concernés. Les anciennes sessions ne
permettent pas de distinguer les connexions Google ayant évité le second facteur.
Les licences, paiements et postes enregistrés ne sont pas supprimés.

### 2. Vérification de l'identité Google et confidentialité des journaux

- Refus de la connexion lorsque l'identifiant client Google n'est pas configuré.
- Vérification stricte de l'audience, du présentateur éventuel, de l'émetteur,
  de l'expiration, de l'identifiant de compte et de l'adresse vérifiée.
- Une adresse tierce sans domaine Google Workspace attesté ne permet plus de
  rattacher automatiquement une connexion Google au compte RelaisDesk. Utiliser
  dans ce cas le mot de passe ou la récupération par e-mail.
- Limitation de la réponse distante et refus des redirections.
- Les erreurs réseau ne journalisent plus l'URL contenant le jeton Google.

Google précise que `email_verified` seul ne garantit pas la propriété actuelle
d'une adresse tierce : [documentation officielle](https://developers.google.com/identity/sign-in/web/backend-auth).

### 3. Clés privées du Viewer et du parc Linux

Des fichiers de preuve privés et jetons étaient créés ou remis en mode `0644`,
notamment dans le répertoire de service accessible aux utilisateurs locaux.
Ils sont désormais écrits en `0600`, y compris lors du renouvellement. Le service
corrige les permissions de ses anciens fichiers après validation de leur
propriétaire, de leurs liens et de leurs chemins.

Les processus du bureau obtiennent les preuves via le socket contrôlé existant,
sans accès direct à la clé privée. La configuration générée ne référence plus
les fichiers secrets. La vérification des fichiers privés refuse maintenant
les droits accordés au groupe ou aux autres utilisateurs.

**Attention aux installations déjà utilisées :** resserrer les permissions ne
révoque pas une clé qui aurait été copiée auparavant. Pour un poste Linux ayant
exécuté la version avec clés lisibles par tous, prévoir la révocation puis le
ré-enrôlement du poste avec une nouvelle identité, particulièrement s'il comporte
des comptes locaux non fiables. Aucune exploitation n'a été constatée par cette
revue locale ; aucune clé active n'a été remplacée à distance.

### 4. Écriture administrateur dans les dossiers Linux des utilisateurs

Le service écrivait en root dans `/home/.../.config/rustdesk/RustDesk2.toml` en
suivant les liens. Un utilisateur local pouvait détourner cette écriture vers
un autre fichier.

La copie utilise maintenant des descripteurs de dossiers, `O_NOFOLLOW`, le contrôle
du propriétaire et un fichier temporaire exclusif remplacé atomiquement. Elle
ne suit pas les liens de dossiers et ne tronque pas les cibles de liens symboliques
ou physiques. Seuls les paramètres générés de connexion sont exportés, pas les
autres options éventuelles du compte root. Le fichier final appartient à
l'utilisateur concerné et reste privé.

### 5. Retour vers une ancienne version par la mise à jour automatique

Une ancienne version correctement signée pouvait être acceptée, et une différence
entre la version demandée et le manifeste ne provoquait qu'un avertissement.

Le service refuse maintenant les versions anciennes, identiques, mal formées ou
différentes de celle demandée, avant téléchargement et installation. Les contrôles
Ed25519 et SHA-256 restent en place. L'écriture du fichier temporaire utilise
une création exclusive et une synchronisation avant remplacement.

Un futur manifeste doit donc correspondre exactement à la version demandée et
être supérieur à la version réellement injectée dans le programme à la compilation.
Un retour arrière volontaire doit rester une opération de maintenance contrôlée,
pas une commande de mise à jour automatique.

### 6. Dépendance TLS des moteurs Rust

L'audit RustSec a trouvé [RUSTSEC-2026-0285](https://rustsec.org/advisories/RUSTSEC-2026-0285)
dans `rustls`, concernant la validation de messages de négociation TLS 1.3.

- Serveur : `rustls 0.23.43` → `0.23.45`.
- Client : `rustls 0.23.28` → `0.23.45`, avec `rustls-webpki 0.103.15`.

Les deux `Cargo.lock` sont mis à jour. L'audit final ne signale plus de
vulnérabilité bloquante dans ces verrouillages avec la base consultée ce jour.
Les binaires déjà construits ne bénéficient pas de ces changements tant qu'ils
n'ont pas été reconstruits et installés.

## Vérifications

- `go -C database test ./...` : réussi, y compris MFA Google et migration unique.
- `go -C api test ./...` et `go -C api vet ./...` : réussis.
- `go -C tests test ./...` : réussi.
- Tests Viewer et configurateur avec `-tags ci` : réussis sous Windows.
- Compilation des tests Viewer pour Linux et `go vet -tags ci` Linux : réussis.
- Exécution réelle dans WSL des tests de permissions, liens, socket de preuve,
  configuration Linux et refus des retours de version : réussie.
- `node --check relaisdesk/client/app.js` et `node tests/google-login-security.cjs` : réussis.
- Serveur Rust, `cargo test --locked --lib relaisdesk_auth -j 2` : 6 tests réussis.
- Moteur client Rust, `cargo test --locked --lib relaisdesk_auth` : 2 tests
  réussis (format des preuves et contrôle d'accès/rejeu côté Viewer).
- `govulncheck` : aucune vulnérabilité appelée détectée dans l'API ; aucune
  vulnérabilité détectée dans le Viewer et le configurateur en configuration CI.
- `cargo audit` après correction, client et serveur : code de sortie 0,
  avec avertissements de maintenance ci-dessous.

## Points résiduels

Complément lors de la préparation du déploiement : le Technicien Linux contrôle
désormais également l'empreinte de `librustdesk.so`. Le lanceur Flutter peut
rester identique entre deux versions et son empreinte seule ne garantit pas que
la bibliothèque contenant les correctifs est effectivement installée. Un test
de régression couvre une empreinte absente, invalide, ancienne et exacte. Les
constructeurs injectent aussi la version dans le Viewer pour le contrôle
anti-retour de version ; la bibliothèque Linux est obligatoire dans le script
de préparation des publications.

RustSec signale encore des bibliothèques non maintenues : notamment `sodiumoxide`,
`bincode`, `ansi_term`, `dlopen_derive`, et plusieurs bibliothèques graphiques ou
de génération de code côté client. Il signale aussi les versions retirées
`chacha20 0.10.1` (serveur) et `spin 0.9.8` (client). Ces avertissements ne sont
pas présentés comme des exploitations démontrées. Leur remplacement demande une
analyse de compatibilité ; ils n'ont pas été masqués dans les scanners.

`govulncheck` signale au niveau du module `golang.org/x/crypto` l'avis
[GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932) sur OpenPGP, sans correctif
annoncé. Le package OpenPGP n'est ni importé ni appelé par l'API analysée ; le
module sert aussi à d'autres fonctions cryptographiques, qu'il ne faut pas retirer
pour faire disparaître artificiellement cet avertissement.

## Fichiers et future mise en service

Pour OVH, transférer ensemble depuis le dossier de référence `relaisdesk/` :

- `client/index.html` (version de cache du script actualisée) ;
- `client/app.js` (traitement du challenge Google).

Les corrections API sont dans `api/handlers/customer.go` et les fichiers
d'authentification de `database/` ; des tests de régression ont été ajoutés.
Elles nécessitent une reconstruction et un déploiement séparés de l'API. À ce
premier démarrage, la migration ci-dessus imposera une reconnexion aux comptes
MFA concernés. L'audit initial n'avait pas déployé cette API. Le déploiement
effectué ensuite, sur autorisation de l'exploitant, est consigné dans
`rapports/DEPLOIEMENT_SECURITE_ORACLE_2026-09-19.md` : API et hbbs/hbbr en
production, programmes 1.0.4 préparés uniquement pour les tests locaux.

Les corrections de programmes sont dans `installer/viewer/` et les verrouillages
`rustdesk/Cargo.lock` / `rustdesk-server/Cargo.lock`. Prévoir leur reconstruction,
les tests de prise en main sur machines de test, puis seulement la distribution
et le renouvellement du manifeste signé. Aucune archive web n'a été créée.
