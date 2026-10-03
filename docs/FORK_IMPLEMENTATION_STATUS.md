# État des forks RelaisDesk

## Révisions publiées

- client : branche `relaisdesk/authorization`, commit
  `c551990c69c95430442569d15b277fb61c6fe0e0` ;
- serveur : branche `relaisdesk/authorization`, commit
  `57318afae9c19935c75e47700a4d69275b71cce8` ;
- protocole client `hbb_common` : branche `relaisdesk/client-protocol`, commit
  `924efac1e2d1002dcad3de51938481bd29d7e46a` ;
- protocole serveur `hbb_common` : branche `relaisdesk/server-protocol`, commit
  `4890b540ceffd2b68305a99bd2aff3df6fb307e3`.

Les trois forks publics sont accessibles sur le compte GitHub `JuLeXoGame`.
Les deux dépôts parents utilisent une URL de sous-module relative et pointent
vers le commit de protocole correspondant à leur rôle.

La clé privée Ed25519 du manifeste de version a été créée hors dépôt dans
`C:\RelaisDesk-Secrets\release-signing-ed25519`, avec un ACL limité à
`Administrators` et `SYSTEM`. Sa clé publique est
`K3k6oko00jMzl7hN3poS6KYjJzZvjNz9Tgdz73E2duo`. La clé privée doit encore être
copiée sur le support de sauvegarde hors ligne du propriétaire.

## Validation effectuée

- `cargo check --locked` du serveur `hbbs`/`hbbr` ;
- 11 tests serveur `hbbs` et 9 tests `hbbr` ;
- `cargo check --locked --lib` complet du client Windows ;
- 93 tests Rust du client, dont la preuve canonique RelaisDesk ;
- tests et `go vet ./...` de l'API, de la base, du configurateur et du viewer ;
- compilation des launchers Linux `amd64` avec injection des empreintes ;
- analyse syntaxique de `installer/build.ps1` ;
- cohérence binaire des deux copies de `rendezvous.proto` ;
- construction release du client Windows et de la DLL d'affichage virtuel ;
- construction et tests des deux launchers Windows avec le fork réellement
  embarqué ;
- validation YAML et `actionlint` du workflow serveur durci.
- nouveau `cargo check --locked` du serveur et `cargo check --locked --lib` du
  client réussis le 2 septembre 2026 sur les commits publiés.
- CI serveur publique entièrement réussie :
  `https://github.com/JuLeXoGame/rustdesk-server/actions/runs/33696687047`.
  Elle a validé les tests, les builds release, Compose sans ports WebSocket,
  l'image Docker durcie, son exécution non-root et les deux exécutables.
- CI client publique entièrement réussie :
  `https://github.com/JuLeXoGame/rustdesk/actions/runs/33675951547`.
  Les 9 jobs utiles ont validé le SBOM, les ponts Flutter, Windows x64/ARM64,
  Linux x64/ARM64, les portables et les deux MSI.
- sauvegarde du projet effectuée par le propriétaire avant la préparation Git
  du poste Windows.

La chaîne Windows est automatisée dans
`scripts/build-rustdesk-windows.ps1`. Les dépendances natives sont épinglées :
baseline vcpkg RustDesk `120deac...`, moteur vcpkg `0ac8df3...`, NASM 2.16.03,
libclang 18.1.1, Brotli 1.2.0 et Sciter au commit `f33df075...` avec contrôle
SHA-256.

## Réactivité vidéo (2026-10-03)

- Core : `INIT_FPS` passé de 15 à 30 dans
  `rustdesk/src/server/video_qos.rs` (le QoS adaptatif continue de baisser
  sur liens lents après les premières sondes).
- Core Windows : build avec `--features inline,vram,hwcodec` (comme l'amont),
  via `scripts/build-rustdesk-windows.ps1`. Le manifeste
  `scripts/rustdesk-sciter-vcpkg/vcpkg.json` ajoute `mfx-dispatch`, `ffmpeg`
  (amf/nvcodec/qsv) et les overrides `ffnvcodec 12.1.14.0` / `amd-amf 1.4.35`.
  Premier build : prévoir ~30 min pour ffmpeg.
- Core Linux : pas de script repo (build manuel). Reconstruire avec :
  `cargo build --locked --release --features inline,hwcodec,unix-file-copy-paste --bins`
  (ligne amont `flutter-build.yml` pour x86_64).
- Lanceurs : preset écrit dans `RustDesk2.toml` (`codec-preference='auto'`,
  `av1-test='N'`, `enable-hwcodec='Y'`). Forçage logiciel :
  `RELAISDESK_DISABLE_HWCODEC=1` ou `--set-hwcodec off` (retour : `on`).
  Le core désactive déjà le matériel seul en cas d'échec (repli VP9).

## Artefacts CI serveur

- `hbbs` Linux amd64 : SHA-256
  `E4E355F8593DA47E7377BBDCE2E282AF5FCBFE69939215C0F92E2CD77DCBE6AF` ;
- `hbbr` Linux amd64 : SHA-256
  `83776BB90C29EF19AFF5E74BC03EFC39AB4DF161972934D17A038A44F63F28AF` ;
- copie de contrôle locale :
  `.cache/relaisdesk-ci-artifacts/57318af/SHA256SUMS.txt`.

Ces artefacts proviennent du commit serveur indiqué plus haut. Ils servent à
la recette et à la comparaison ; le déploiement Oracle doit rester construit
depuis le commit public épinglé par `Dockerfile.relaisdesk`.

## Artefacts CI client x64

Les sorties contrôlées du run `33675951547` sont conservées sous
`.cache/relaisdesk-ci-artifacts/client/run-33675951547`. Les empreintes
complètes sont dans `SHA256SUMS.txt` :

- portable Windows : SHA-256
  `EC9E3ACB5117555F067AA5B7A1BA4A28852FBA8BEC7EE3860BFCE650C7EF4493` ;
- MSI Windows : SHA-256
  `1466A630FE1312E0794E6D76E2540D14A8D4B5F5C59498FB13A90568EFB5C803` ;
- binaire Flutter brut `RelaisDesk.exe` : SHA-256
  `DE185085FA7086439334298E10F12CE4CF05733F808C8FE9141BD45A72319E03` ;
- paquet Debian : SHA-256
  `B2C638DB41306198D037C64C3C0E83F5456C2AD0ADD8632530F56A1F3D981E79` ;
- ELF installé `/usr/share/rustdesk/rustdesk` : SHA-256
  `58EF1E984727D827836C8AD84AD50A4971DB4A3CB80AD7A646982594AF155C52` ;
- SBOM CycloneDX 1.6 (1 589 composants) : SHA-256
  `9677FF66DB583A68D168D5BD1C88C39C7F87DBB26B184E796392876B1B07E50C`.

Le lanceur ELF et `librustdesk.so` ont l'en-tête ELF64, machine 62
(`x86_64`). Le MSI expose `ProductName=RelaisDesk`, `ProductVersion=1.4.9.*`
et `Manufacturer=Julien BELLOT EI`. L'EXE brut expose le produit RelaisDesk et
la version `1.4.9+67` dans ses ressources Windows.

Les EXE, le MSI et `librustdesk.dll` sont **non signés**, comme attendu avant
SignPath. Ces fichiers sont utilisables pour une recette isolée et la
candidature, pas pour une publication client.

## Artefacts de recette locaux

La recette `installer/build.ps1` a réussi avec le portable et le DEB du run CI
client. Les six fichiers réellement proposés par le site sont :

- `RelaisDesk_Portable.exe` :
  `2e060cb120207f39a4b4d50cac33176a6565132a5e22e42273098fcd3b94f7f1` ;
- `RelaisDesk_Setup.exe` :
  `6b1a236c209645b1253f4fe690e000c2d90f7e6195ac61a26ed4c45b508215e8` ;
- `RelaisDesk_Technicien_Portable.exe` :
  `1ef3c46ab5186a2fa1d942e5d35a5ea3509722944bfe5c9f97fd6954ebe2ce0b` ;
- `RelaisDesk_Technicien_Setup_1.0.0.exe` :
  `0f5468c2dc8b372b0e8aa94e90c0e749736746ec764b7681ac06aeccfe7e4d0c` ;
- `RelaisDesk_Technicien.deb` :
  `eee2169ef8971e3127c7a4b962c1abebb5717f9c64b95bf0f846800ce4e23abb` ;
- `RelaisDesk_viewer.deb` :
  `6252e949825f8306245a77b75e117c7e6d94c93fc03bbdf4b2f111bc7b5de0d6`.

`SHA256SUMS.txt` contient exactement ces six entrées et elles ont toutes été
recalculées avec succès. `release-manifest.json`, signé et auto-vérifié avec la
clé `release-1`, contient ces six fichiers plus `SHA256SUMS.txt`. Il ne contient
ni `.htaccess`, ni lui-même, ni fichier hérité d'une ancienne recette.

Le script vérifie désormais les formats PE, DEB et ELF64 x86_64 plutôt qu'une
taille minimale erronée pour le petit lanceur Flutter Linux. Il utilise aussi
une liste blanche explicite des sorties. L'ancien
`rustdesk-1.4.9-x86_64.exe`, non référencé par le site et différent du build CI
courant, a été retiré de `downloads` et placé de façon récupérable dans
`.cache/relaisdesk-quarantine/stale-downloads`.

Les quatre EXE RelaisDesk de recette sont **non signés**. La publication cible
doit reconstruire et signer la chaîne avec SignPath, puis recalculer les
empreintes et le manifeste. Microsoft Defender n'a pas pu analyser les
fichiers sur ce poste (`Provider load failure`) : aucun succès antivirus n'est
revendiqué.

## Blocages restants avant publication des binaires

Docker et WSL ne sont pas installés sur le poste. GitHub CLI est authentifié sur
le compte `JuLeXoGame`, les branches sources sont publiques et les CI client et
serveur sont validées. Il reste :

- à publier sous licence OSI le dépôt distinct des configurateurs, viewers et
  installateurs requis pour la candidature SignPath ;
- à obtenir l'acceptation SignPath et à produire les artefacts signés.

Le workflow serveur `.github/workflows/relaisdesk-ci.yml` est validé : il teste
les deux serveurs, valide Compose, construit l'image avec des bases épinglées,
contrôle l'utilisateur non-root et publie les deux binaires Linux comme
artefacts du run.

Avant toute production :

1. sauvegarder hors ligne la clé de version déjà créée selon
   `RELEASE_SIGNING.md` ;
2. utiliser les artefacts du run client réussi pour appeler
   `installer/build.ps1` avec cette clé, sa clé publique, le portable Windows,
   le DEB et le binaire ELF extrait de ce DEB ;
3. publier le dépôt de distribution requis et obtenir l'acceptation SignPath ;
4. signer les exécutables Windows par la chaîne SignPath retenue ;
5. effectuer une recette de plus de cinq minutes couvrant P2P, relais,
   renouvellement, déconnexion technicien, révocation viewer et dépassement de
   quota ;
6. mettre à jour les liens vers les commits correspondant exactement aux
   artefacts signés avant de distribuer les binaires ou d'exposer le service
   modifié.

Le déploiement de production n'a pas été exécuté depuis ce workspace.
