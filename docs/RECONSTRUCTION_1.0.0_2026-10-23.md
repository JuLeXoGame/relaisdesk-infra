# Sources de la reconstruction RelaisDesk 1.0.0 du 23 octobre 2026

Cette reconstruction conserve le numéro produit 1.0.0. Les empreintes du manifeste
signé identifient les fichiers ; le numéro seul ne distingue pas les lots.
La clé privée de signature, les accès Oracle et les données métier ne sont pas
nécessaires pour compiler les sources. Ils ne sont pas inclus dans les dépôts.

## Révisions

- Lanceurs Go et scripts : dépôt `JuLeXoGame/relaisdesk-infra`, branche
  `codex/security-perf-1-0-20261023`. Les changements de code d'assemblage et de
  création Debian sont inclus dans le commit `01f72bb`. Le commit `1cb44f9`
  ajoute l'assemblage macOS conservant le moteur dans son application signée.
- Moteur client : `JuLeXoGame/relaisdesk`, commit
  `1054d0f09e4e1d4aaea1309af714289e6055ba4b`.
- Serveur : `JuLeXoGame/rustdesk-server`, commit
  `adca9567eaf9e865ac2085877c953856016d1c68`.

Pour les deux dépôts Rust, initialiser les sous-modules avec
`git submodule update --init --recursive`. Conserver les fichiers Cargo.lock et
go.sum. Les archives automatiques GitHub seules ne contiennent pas les sous-modules.

## Dépendances natives et moteur

Go 1.26.10 est utilisé pour les lanceurs et outils. Le fork client contient les
ports vcpkg AOM 3.14.1 et FFmpeg 7.1.5 et les correctifs appliqués à leurs sources.
`tests/aom_svc_bounds.c` vérifie les bornes des identifiants SVC du codec AV1.

Windows utilise `scripts/build-rustdesk-windows.ps1` avec le manifeste
`scripts/rustdesk-sciter-vcpkg`. Les options du moteur incluent `inline`, `vram` et
`hwcodec`. Le reçu JSON de ce script contient le chemin et l'empreinte du moteur
portable et du moteur natif utilisé pour le service du Viewer.

Linux utilise la bibliothèque Rust construite avec
`cargo build --locked --lib --release --features flutter,unix-file-copy-paste,linux-pkg-config`.
AOM 3.14.1 est compilé statiquement, avec PIC. VPX et libyuv sont également liés
statiquement depuis un préfixe privé pkg-config. Dans l'environnement Ubuntu
22.04 utilisé, VPX provient du paquet corrigé `1.11.0-2ubuntu2.5`. La bibliothèque
`liblibrustdesk.so` remplace `usr/share/rustdesk/lib/librustdesk.so` du paquet
embarqué, puis reçoit le RPATH `$ORIGIN`. Le lanceur Flutter et ses ressources
existants sont conservés. Les fichiers de pont Rust générés ne sont réutilisés
que lorsque `src/flutter_ffi.rs` est identique ; un nouvel environnement peut les
régénérer avec flutter_rust_bridge_codegen 1.80.1 et cargo-expand 1.0.95, comme
dans le workflow macOS. Vérifier `ldd -r` du lanceur Flutter et de la bibliothèque,
et l'absence de dépendances dynamiques AOM, VPX et libyuv.

macOS est construit par `.github/workflows/build-macos.yml`, pour ARM64 et Intel.
Le workflow fixe le commit client et les dépendances, régénère le pont Flutter,
construit les lanceurs et conserve l'application native complète dans
`Contents/Helpers/RelaisDeskEngine.app` de chacun d'eux. Ce moteur garde son propre
Info.plist, ses frameworks, ressources, service et sa signature. Le lanceur cherche
ce chemin et vérifie l'empreinte du moteur avant exécution. Le script commun
`scripts/package-macos-verified-engine.sh` crée et monte les deux DMG pour vérifier
leur contenu, les architectures, les empreintes et les signatures ad hoc.
Le workflow `package-macos-verified-engine.yml` reprend les moteurs déjà compilés
de l'exécution 39916359240, dont il vérifie le commit producteur et les signatures,
pour terminer cette reconstruction. Il ne fournit pas de signature Developer ID
ni de notarisation Apple.
Les minimums système sont macOS 12.3 ARM et macOS 12.0 Intel.

## Lanceurs et paquets 1.0.0

Pour Windows, appeler `scripts/build-windows-test-release.ps1` avec les chemins
et empreintes du reçu moteur, un nouveau dossier de travail et un nouveau dossier
de sortie, la clé **publique** du manifeste et `-Version 1.0.0`. NSIS et un
compilateur C sont nécessaires. Les deux modules sont testés avant assemblage.

Pour Linux, `scripts/build-linux-test-release.sh` reçoit le paquet moteur,
ses empreintes DEB/ELF/bibliothèque, la clé publique, deux nouveaux dossiers et
`--version 1.0.0`. Le Technicien graphique requiert CGO et les en-têtes GL/X11 ;
le Viewer est compilé avec CGO désactivé. Exécuter ensuite les deux binaires de
tests produits depuis leurs dossiers `sources/viewer` et `sources/configurator`.
Le paquet du Technicien déclare ses dépendances GL/X11 et glibc.

`scripts/stage-launcher-release.py` copie ensemble les deux modules et leurs
fichiers de contrôle. Les empreintes des copies correspondent au nouveau moteur,
sans écraser les moteurs embarqués du dossier de développement. Les tests
comparent réellement les empreintes des paquets et de leur contenu.

La signature du manifeste de publication utilise `api/cmd/release-manifest`.
Une reconstruction indépendante peut employer sa propre clé de test ; elle ne
doit pas prétendre être signée par la clé officielle RelaisDesk.
