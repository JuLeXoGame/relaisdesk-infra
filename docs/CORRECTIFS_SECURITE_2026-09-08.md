# Correctifs de sécurité — 8 septembre 2026

## État de livraison

- Sources client corrigées et publiques : [e34c8bab932791961b2406bac5d0256b744b40b1](https://github.com/JuLeXoGame/relaisdesk/commit/e34c8bab932791961b2406bac5d0256b744b40b1).
- Sources serveur corrigées et publiques : [d998a326afc32bbd26614e84f2b3332e8fdd10ab](https://github.com/JuLeXoGame/rustdesk-server/commit/d998a326afc32bbd26614e84f2b3332e8fdd10ab).
- Les 25 alertes Dependabot client et 10 alertes serveur initiales sont closes automatiquement après analyse des nouveaux manifestes. Aucune clôture manuelle ni exclusion globale d'avis RustSec.
- Serveur Oracle mis à jour et vérifié le 7 septembre à 23:40 UTC (8 septembre à 01:40, heure de Paris).
- Clients Windows/Linux : les six paquets `1.0.1` sont construits et signés via leur manifeste, **uniquement en local** dans `output/security-2026-09-08/release-1.0.1/downloads`. Linux x64 provient du [build 34169303850](https://github.com/JuLeXoGame/relaisdesk/actions/runs/34169303850). Windows a été reconstruit localement en **Sciter**, comme le moteur actuellement distribué ; le paquet Flutter Windows de ce workflow n'est pas choisi pour la livraison. La signature Ed25519 et les sept empreintes ont été vérifiées indépendamment ; les installateurs Windows contiennent exactement les nouveaux lanceurs.
- **Publication suspendue à la demande de l'utilisateur : tests manuels puis accord explicite nécessaires. Aucun transfert ni activation des nouveaux paquets clients.** La dernière lecture de l'API publique confirme `1.0.0-legal.20260907`. La correction des sources ne corrige pas rétroactivement les anciens exécutables en ligne. Voir [la procédure de recette](RECETTE_SECURITE_1.0.1.md).
- OVH : aucun accès de déploiement disponible ; la page `logiciel-libre.html` est préparée localement, mais ne doit pas être présentée comme publiée. Aucune modification d'OVH, du DNS ou des certificats TLS Oracle n'a été effectuée pendant cette livraison.

## Corrections

| Périmètre | Correction et validation |
| --- | --- |
| Client OpenSSL | Crate 0.10.81, bibliothèque embarquée 3.6.3 ; les plateformes qui la compilent doivent être reconstruites. |
| Presse-papiers Linux | fuser 0.16.0 ; tests du composant FUSE réussis. |
| Génération aléatoire | Branches rand 0.8.8 / 0.9.5. L'ancien rand 0.7 de l'interface serveur est retiré via une adaptation PHF testée. |
| Terminal Windows/Linux | Remplacement local d'atty par une petite API compatible utilisant `std::io::IsTerminal`, sans `unsafe`, testée avec terminal redirigé. |
| GLib / GTK3 | Reprise du correctif officiel d'itérateur dans les versions compatibles ; sources complètes, licences, provenance et tests optimisés inclus. |
| Exemple de capture client | Suppression de quest/rpassword ; lecture standard avec refus d'écrasement à la fin du flux ou en cas d'erreur. |
| Lockfiles secondaires | Retrait des deux anciens lockfiles portable/virtual_display : ces membres utilisent réellement le lockfile racine. Les fichiers restent récupérables dans Git. |
| Pool SQLite serveur | Désactivation du chargement de configuration deadpool inutilisé, retirant nom 5 / lexical-core 0.7. |
| Interface serveur historique | Tauri 1.8.3, Vite 6.4.3, lockfiles actualisés ; serveurs de développement/prévisualisation sur 127.0.0.1. Correction du double lien de ressources Windows après migration Tauri. Cette interface n'est pas exécutée sur Oracle. |
| Clé RustDesk Oracle | Dossier des données passé de 0755 à 0700 ; clé privée et base SQLite de 0644 à 0600. Propriétaires UID/GID 10001 conservés. La clé était lisible par d'autres comptes locaux ; cela n'établit pas une compromission distante. La clé n'a pas été changée ni copiée hors du serveur. |
| Maintenance Oracle | Suppression des anciens blocs `build` du Compose de production : un `up --build` ultérieur ne doit pas reconstruire les anciens fichiers sous le nouveau tag. Ajout d'un `.dockerignore` au contexte de préparation pour exclure les sauvegardes privées. Aucun redémarrage supplémentaire. |

Les corrections locales gardent leur numéro de compatibilité amont. Elles ne sont pas présentées comme de nouvelles versions officielles. `tools/check_security_dependencies.py` vérifie leur utilisation effective et leurs empreintes, car un scanner de versions seul ne contrôle pas leur contenu.

## Tests vérifiables

- [Sécurité client — succès, 4 jobs](https://github.com/JuLeXoGame/relaisdesk/actions/runs/34169302556) : audit, atty Windows/Linux, GLib optimisé, presse-papiers/FUSE.
- [Sécurité serveur — succès, 3 jobs](https://github.com/JuLeXoGame/rustdesk-server/actions/runs/34169496183) : audit, atty, GLib optimisé, compilation et test PHF de l'interface Windows.
- [Construction complète serveur — succès](https://github.com/JuLeXoGame/rustdesk-server/actions/runs/34169496112) : autorisations hbbs/hbbr, binaires, image Docker et contrôles Compose.
- [Construction complète client Flutter Windows/Linux — succès](https://github.com/JuLeXoGame/relaisdesk/actions/runs/34169303850). Le moteur Windows Sciter distribué est toutefois reconstruit séparément, sans changer d'interface.
- Client Sciter local : 93 tests de bibliothèque réussis avant construction optimisée. Parmi eux, le test amont `test_uninstall_cert` appelle une opération système réelle ciblant l'ancien certificat de test WDK `D1DBB672D5A500B9809689CAEA1CE49E799767F0` et des magasins erronés. Il a été exécuté par le script existant ; les sorties ne permettent pas d'établir si ce certificat était présent puis supprimé. Aucune suppression de magasin parasite n'est apparue dans les sorties. Ce test est désormais exclu du script de construction courant : il ne doit être exécuté que sur une machine jetable. Aucun certificat n'a été réinstallé pour tenter de compenser une suppression non établie.
- Nouvelle exécution du binaire de tests Sciter avec cette exclusion : **92 réussis, aucun échec, 1 filtré**. Tests Windows du configurateur et du viewer Go avec `-trimpath -mod=readonly` réussis.
- Les quatre nouveaux lanceurs Go (technicien/client, Windows/Linux) passent `govulncheck -mode=binary` sans vulnérabilité connue. Les exécutables de tests Linux ont été compilés, mais pas exécutés sur Linux dans cette recette locale.
- Audits locaux sur la base RustSec `8a1eb4f933fb5821add5b4e98601ebd90b8b3538` : aucune vulnérabilité ni alerte « unsound » sur les trois lockfiles actifs. Les avis de non-maintenance restent visibles.
- Interface serveur : `npm audit` sans vulnérabilité ; construction Vite et liaison Windows réussies.
- API Go : `govulncheck -show verbose ./...` ne trouve aucune vulnérabilité dans les symboles appelés ni les paquets importés. L'avis GO-2026-5932 concerne uniquement `golang.org/x/crypto/openpgp`, non importé par l'API ; le module x/crypto reste nécessaire pour les autres paquets utilisés.

## Oracle : version active et retour arrière

Image : `relaisdesk/rustdesk-server:d998a32-security-amd64`.

Identifiant : `sha256:396e3556158404b4ae9fbe087e2b2de00a4af6e73524a5bffc406741d165e14d`.

SHA-256 des binaires actifs :

```text
64d0bb019610c8cb9588e583894db857de61e347621965221aa65449d859a503  hbbs
ba899e46733bcccde1d9d7166c13627b7b6ec3bae6a0ef1b89b75c33e5c17e96  hbbr
```

Activation après contrôle d'absence de connexion TCP RustDesk établie. Les vérifications ont confirmé : conteneurs actifs sans redémarrage en boucle, utilisateur 10001:10001, système racine en lecture seule, ports TCP 21115–21117 accessibles localement, 21118/21119 non publiés, identité RustDesk inchangée, API saine et base connectée. `RELAISDESK_AUTH_REQUIRED=Y` est conservé. Le binaire et le fichier d'environnement API n'ont pas été modifiés lors de cette activation.

Sauvegarde privée sur Oracle : `/opt/relaisdesk/deployments/security-server-20260908-zpfqos/rollback`. Elle contient l'ancien Compose, son environnement, les clés locales et une sauvegarde SQLite cohérente vérifiée. Ne pas télécharger ni publier ce dossier. L'ancienne image `57318af-amd64` est conservée.

En cas de besoin, rétablir uniquement les fichiers Compose et environnement sauvegardés, puis relancer les services avec l'ancienne image déjà présente. Ne pas restaurer automatiquement la base : cela supprimerait des écritures intervenues depuis la sauvegarde. Aucune migration du protocole ni du schéma RustDesk n'est introduite par cette mise à jour.

## Limites et suite

Les tests automatisés ne remplacent pas une recette entre deux ordinateurs : connexion technicien/client, maintien de session au-delà de cinq minutes, presse-papiers, transfert de fichiers et contrôle de la limite simultanée. Aucun résultat positif de cette recette réelle n'est revendiqué ici.

Les bibliothèques non maintenues (notamment GTK3 et sodiumoxide) restent une dette technique. Zéro alerte connue à un instant donné n'est pas une garantie d'absence de toute faille.

Une mise à jour du site de téléchargement ne remplace pas automatiquement les copies déjà installées chez les techniciens ou clients. Ceux-ci doivent utiliser les nouveaux lanceurs pour recevoir le moteur corrigé.

## Préparation des clients

- Linux x64, commit client ci-dessus : paquet `rustdesk-1.4.9-x86_64.deb`, SHA-256 `9bf84efaa49eb7497df7d26ddb5333385bb390badddd04cfe8f8d611ae0f31c1` ; moteur ELF `usr/share/rustdesk/rustdesk`, SHA-256 `58ef1e984727d827836c8ad84ad50a4971db4a3cb80ad7a646982594af155c52`.
- Windows : `scripts/build-rustdesk-windows.ps1 -SkipNativeDependencies -SkipLaunchers -DestinationPath <nouvel-exe>` reconstruit le moteur et l'emballage Sciter sans remplacer les téléchargements. L'option de destination ne permet pas d'écraser un fichier existant.
- Moteur Windows Sciter produit : `output/security-2026-09-08/client/windows-sciter/RelaisDesk-RustDesk-1.4.9-windows-x64.exe`, SHA-256 `86a2132b04743e3c2199cc20a86e158412bcb549e4e9ab247427a9a463f326c0`. Le moteur est intégré aux deux lanceurs Windows de test.
- `scripts/build-security-release.ps1` crée un répertoire neuf, compile les deux lanceurs et les six paquets en version `1.0.1`, puis signe le manifeste avec la clé privée locale existante. La clé privée ne quitte pas Windows.
- `scripts/verify-security-release.mjs` contrôle indépendamment avec Node la signature, les sept empreintes, les formats des six paquets et `SHA256SUMS.txt`.
- Le nom de téléchargement `RelaisDesk_Technicien_Setup_1.0.0.exe` reste stable pour ne pas casser les liens existants ; la version de l'installateur est `1.0.1`. Le script de construction général accepte maintenant `-Version` et transmet la même version à NSIS.
- Manifeste local `1.0.1` : SHA-256 `fde314951e4352dc5e124df203d54aab59b74a33495dd5623754ef2c5bea2604`. Il n'est pas le manifeste actuellement servi par l'API. Les scripts de préparation/activation des téléchargements ne doivent pas être exécutés avant validation explicite.

Les paquets préparés et les fichiers OVH locaux ne sont pas une preuve de publication. Seule la section « État de livraison » indique ce qui est réellement actif.

Les modèles `scripts/docker-compose-oracle-prebuilt.yml` et `scripts/Dockerfile.relaisdesk-prebuilt-amd64`, le nouveau modèle `.dockerignore` et la section serveur du guide de migration ont été actualisés. Le Dockerfile épingle maintenant les empreintes des binaires de la CI, pas seulement leur étiquette de commit. Les modèles ont été copiés et testés sur Oracle dans le sous-dossier privé `templates` du déploiement serveur. L'image de test `d998a32-template-check` est distincte de l'image active ; sa construction et les deux contrôles SHA-256 ont réussi. Aucune donnée ni clé n'est incluse dans son contexte Docker.
