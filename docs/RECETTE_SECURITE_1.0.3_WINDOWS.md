# RelaisDesk Windows 1.0.3 — préparation du 12 septembre 2026

Les quatre exécutables corrigés sont disponibles dans `installer/build/test-securite-1.0.3/`. **Lot local à tester, non publié**. Aucun exécutable client sur Oracle/OVH ni aucun fichier de `relaisdesk/downloads/` n'a été remplacé. Aucun ZIP et aucun manifeste de publication n'ont été créés. Les paquets Linux restent inchangés.

## Fichiers

| Usage | Fichier | Taille |
| --- | --- | --- |
| Poste assisté | `RelaisDesk_Portable.exe` | 77 608 960 octets |
| Poste technicien | `RelaisDesk_Technicien_Portable.exe` | 38 013 440 octets |
| Installation des deux programmes | `RelaisDesk_Setup.exe` | 55 139 664 octets |
| Installation du technicien seul | `RelaisDesk_Technicien_Setup_1.0.3.exe` | 22 341 170 octets |

Le nom du dernier fichier identifie explicitement cette version de test ; il ne remplace pas le nom historique utilisé par les liens publics. Le Viewer est plus volumineux car il intègre aussi le moteur natif et ses deux DLL pour l'accès permanent sécurisé.

SHA-256 :

```text
67a8648a5e9e1545c63f479c2908219e63b139786e0e0003e61fa511860ec0b8  RelaisDesk_Portable.exe
7d99d8542e21b1f0b156afd3994e9eaeef5b6866fefda5b86d4255624b027992  RelaisDesk_Setup.exe
f85d30241b42d3bf0b2552defc1019dbe6fbe49f7c2bd64f4aad1716fd5f777c  RelaisDesk_Technicien_Portable.exe
65d826823e58a24ef9ade4353d570d3dd6f6ec518959a0d612d2e85498e117d5  RelaisDesk_Technicien_Setup_1.0.3.exe
```

## Corrections incluses

Sources Go actuelles : DPAPI par utilisateur pour les identifiants, migration du stockage ancien lors de sa lecture, moteur permanent embarqué et vérifié avec ses DLL, contrôle du dossier protégé et respect d'un service RustDesk indépendant. Les corrections API ont déjà été [déployées sur Oracle](rapports/DEPLOIEMENT_SECURITE_ORACLE_2026-09-12.md).

Un écart a été constaté dans la préparation 1.0.2 : les fichiers `installer/viewer/embedded/rustdesk.exe` et `installer/configurator/embedded/rustdesk.exe`, ainsi que l'identité enregistrée dans le Viewer 1.0.2, désignent `b73131f36cec9e8e54446e009b58981362d20a7a3024886d7813b6ca836e52d5`, et non le moteur corrigé documenté le 8 septembre. La notice de ce lot ne suffit donc pas à prouver qu'il contient ce moteur corrigé.

Le lot 1.0.3 utilise explicitement l'artefact du [lot de sécurité du 8 septembre](CORRECTIFS_SECURITE_2026-09-08.md), correspondant au fork `e34c8bab932791961b2406bac5d0256b744b40b1` d'après son compte rendu de construction :

- Wrapper portable : `86a2132b04743e3c2199cc20a86e158412bcb549e4e9ab247427a9a463f326c0`.
- Moteur natif `rustdesk.exe` : `f67cbc42a91f3bc4bfded8409937bca9a3c815d47c7163ba7f1302a630593ac7`.
- `sciter.dll` : `4d97528e157c55ef1fabe9e37a9697116ab66660d7da6163f90a3a7abf80dd56`.
- `dylib_virtual_display.dll` : `5eaab07d5a200c3ff679fe37f6380b1540af70d03e21e12f07a8f34920b4ac7a`.

L'artefact a été décodé hors ligne comme des données après contrôle de son SHA-256, sans exécuter son wrapper ni utiliser un cache d'extraction utilisateur. Le natif extrait correspond aussi à l'empreinte du résultat de compilation Rust local du 8 septembre. Aucun code du fork Rust n'a été modifié pendant cette préparation ; les modifications locales non publiées spécifiques à Linux restent intactes.

## Vérifications effectuées

- Compilation réelle Windows amd64 avec Go 1.26.6, CGO et l'interface graphique native, pas avec le moteur graphique simulé des tests.
- Tests et `go vet` du Technicien et du Viewer sur les sources préparées, en mode de test `ci`. Vérification positive du paquet natif et de l'empreinte injectée lors du build ; tests DPAPI, intégrité DLL et refus du cache non vérifié.
- Analyse `govulncheck -mode=binary` des deux portables contre la base officielle Go : aucune vulnérabilité connue signalée. Cette analyse ne couvre pas les bibliothèques Rust/C/C++ embarquées et ne garantit pas l'absence de toute faille.
- Formats PE et sous-système Windows GUI des portables, dépendances DLL système, empreintes SHA-256 vérifiés.
- Installateurs NSIS ouverts avec 7-Zip, sans exécution : les programmes qu'ils contiennent sont identiques aux portables et les deux notices de licence correspondent aux fichiers du projet.
- Vérification indépendante de la présence exacte du wrapper contrôlé dans les deux portables, et des trois fichiers natifs contrôlés dans le Viewer.
- Extraction d'un wrapper présentant un mauvais SHA-256 refusée avant création du dossier cible.
- Cohérence et signature Ed25519 du lot local existant 1.0.2 revérifiées : ses exécutables et son manifeste sont conservés à l'identique.

Les quatre exécutables sont **sans signature Authenticode**. Aucun certificat n'a été acheté/utilisé ; aucune clé privée de signature n'a été nécessaire pour ce lot de test. Le Technicien conserve la clé publique de vérification du canal de mise à jour et porte la version applicative 1.0.3.

## Essais à effectuer avant publication

Utiliser deux PC ou VM de test avec des comptes autorisés. Ces programmes contactent l'API de production : les essais peuvent apparaître dans l'historique et réserver des connexions. Les portables ne sont pas isolés et peuvent modifier la configuration RustDesk locale ; ne pas tester pendant une intervention importante.

1. Commencer avec les deux portables : connexion, saisie 2FA si configurée, affichage, clavier/souris, presse-papiers et transfert d'un fichier sans données personnelles.
2. Maintenir une session interactive au moins dix minutes, puis fermer, se déconnecter et se reconnecter.
3. Fermer et rouvrir le Technicien pour vérifier la mémorisation chiffrée, sans copier les identifiants dans un message ou une capture.
4. Sur une VM Windows jetable, tester l'accès permanent : installation avec élévation, mot de passe permanent, connexion autorisée, redémarrage Windows, révocation, désinstallation. Si un autre service RustDesk existe, son remplacement doit être refusé ; ne pas le supprimer sans identifier son propriétaire.
5. Tester ensuite les deux Setup : installation, raccourcis, démarrage et désinstallation. Ne pas désactiver globalement Defender ou SmartScreen en cas d'avertissement.

Une ancienne installation dans `Program Files\RustDesk` ne migre pas automatiquement vers le dossier protégé `Program Files\RelaisDeskEngine`. Une intervention contrôlée peut être nécessaire ; les tests n'ont pas modifié une installation Windows réelle.

La publication des fichiers et d'un nouveau manifeste reste subordonnée aux essais et à un accord explicite. Aucun résultat positif de connexion distante, d'installation permanente ou de redémarrage Windows n'est revendiqué ici.

## Reconstruction

`scripts/build-windows-test-release.ps1` prépare les deux sources dans un dossier neuf, impose les SHA-256 des entrées, embarque les composants via `scripts/prepare-fleet-payload.ps1`, teste et compile, puis construit les Setup dans un dossier neuf hors du site. Il ne publie pas et ne remplace pas un lot existant.

Sources de ce build : `.cache/windows-security-20260912/build-1.0.3/sources/`. Composants natifs vérifiés : `.cache/windows-security-20260912/reviewed-native/`. Contrôles d'extraction NSIS : `.cache/windows-security-20260912/qa-setup/` et `qa-technicien/`.
