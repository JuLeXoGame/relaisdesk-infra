# Sources de la reconstruction RelaisDesk 1.0.0 du 29 octobre 2026

Le numéro produit reste **1.0.0**. La date et les empreintes du manifeste signé
distinguent cette reconstruction des lots précédents. Les douze programmes sont
construits depuis les sources corrigées, dont le choix des dossiers du parc dans
le Technicien et les mises à jour des dépendances Rust et Go.

## Révisions de sources

| Composant | Dépôt et révision |
| --- | --- |
| Lanceurs Go, API et scripts | [relaisdesk-infra, 0d7a40722967dd3b493af1799e03e171588f3d31](https://github.com/JuLeXoGame/relaisdesk-infra/tree/0d7a40722967dd3b493af1799e03e171588f3d31) |
| Moteur client, sources fusionnées | [relaisdesk, a5043e4ed97c80412d25c39a0d91b8be4c402be4](https://github.com/JuLeXoGame/relaisdesk/tree/a5043e4ed97c80412d25c39a0d91b8be4c402be4) |
| Serveur hbbs/hbbr | [rustdesk-server, b5934ee107c26035877119da195bf04746804944](https://github.com/JuLeXoGame/rustdesk-server/tree/b5934ee107c26035877119da195bf04746804944) |

Les constructions locales Windows et Linux utilisent la révision client
`421877211d56733097ae5585db25d92386434ed4`. Le workflow macOS fixe
`2c074a088c4980562a66d2788c7cdd744f4b0211`. Leur code applicatif et leurs
dépendances sont identiques à ceux de `a5043e4` ; les changements supplémentaires
concernent la CI, ses tests et ses outils. Les sources Go des exécutables locaux
correspondent à `de0b6b5a912b94c4c16567f167fb8a29d30583bc` ; `0d7a407` ajoute
l'installation de `rustfmt` nécessaire au générateur de pont macOS.

Initialiser les sous-modules avec `git submodule update --init --recursive`.
Conserver `Cargo.lock`, `go.sum` et les ports natifs du fork. Le client utilise
le sous-module `hbb_common` à la révision `388c6d3`, qui contient la migration
du transport WebRTC facultatif et de la compression. Les programmes distribués
n'activent pas la fonctionnalité Cargo WebRTC.

## Construction

Les [instructions du 23 octobre](RECONSTRUCTION_1.0.0_2026-10-23.md) décrivent
l'assemblage des moteurs, des lanceurs et des installateurs. Pour ce lot :

- utiliser Go **1.26.10**, Rust **1.99.1**, AOM **3.14.1**, FFmpeg **7.1.5**
  et Brotli Python **1.2.1** pour le générateur portable Windows ;
- Windows : `scripts/build-rustdesk-windows.ps1`, puis
  `scripts/build-windows-test-release.ps1 -Version 1.0.0` avec les chemins et
  empreintes du reçu moteur et de nouveaux dossiers de travail et de sortie ;
- Linux : construire la bibliothèque Rust avec les fonctionnalités
  `flutter,unix-file-copy-paste,linux-pkg-config`, remplacer celle du paquet natif,
  fixer son RPATH à `$ORIGIN`, actualiser ses dépendances de base et recalculer
  les empreintes du DEB, du lanceur et de la bibliothèque ; appeler ensuite
  `scripts/build-linux-test-release.sh --version 1.0.0` avec ces empreintes ;
- le paquet natif Linux conserve le lanceur Flutter et ses ressources. Sa
  révision interne est `1.4.9-1relaisdesk20261029` ; les applications RelaisDesk
  et leurs paquets restent en **1.0.0**. Exécuter les deux binaires de tests
  générés depuis leurs dossiers `sources/viewer` et `sources/configurator` ;
- macOS : lancer `.github/workflows/build-macos.yml` à la révision `0d7a407`,
  avec l'entrée `version=1.0.0`. Ce workflow reconstruit les moteurs ARM64 et
  Intel, leurs lanceurs et les quatre DMG. `rustfmt` doit être installé dans la
  chaîne Rust utilisée pour générer le pont Flutter ;
- dans un environnement où Git ne peut pas inspecter le dépôt parent, utiliser
  `GOFLAGS=-buildvcs=false` pour désactiver seulement cet estampillage facultatif ;
- serveur : `cargo test --locked --lib --bin hbbr`, puis
  `cargo build --locked --release --bins`. Le Dockerfile du fork précise la
  base Debian et l'utilisateur non privilégié.

Les scripts injectent les empreintes du moteur dans les lanceurs. Vérifier
les charges utiles des installateurs NSIS et des paquets Debian, `ldd -r` sous
Linux et le test `tests/aom_svc_bounds.c`. L'assemblage macOS préserve le moteur
dans `Contents/Helpers/RelaisDeskEngine.app` et vérifie les DMG montés.
Les signatures macOS sont ad hoc, sans notarisation Apple. Les exécutables
Windows ne revendiquent pas de signature Authenticode.

Le manifeste est créé par `api/cmd/release-manifest`, avec la liste exacte des
douze programmes et de `SHA256SUMS.txt`. La clé privée officielle, les accès
serveur et les données de production ne font pas partie des sources. Une
reconstruction indépendante peut utiliser une clé de test et doit l'identifier
comme telle. Les empreintes officielles figurent dans le
[manifeste courant](https://api.relaisdesk.fr/api/v1/public/releases/latest).

La version restant 1.0.0, une application installée qui compare uniquement le
numéro de version peut nécessiter un remplacement manuel pour recevoir ce lot.
