# Alertes révélées par l'activation de Dependabot

> Mise à jour du 8 septembre 2026 : les correctifs techniques ont maintenant
> été publiés et le serveur Oracle mis à jour. Voir le
> [rapport de correction et l'état exact des binaires](CORRECTIFS_SECURITE_2026-09-08.md).
> Le texte ci-dessous conserve le constat initial, avant cette intervention.

Constat initial du 7 septembre 2026 : les alertes de dépendances n'étaient
pas activées sur les forks. Leur activation a révélé **25 alertes client**
(8 élevées, 7 modérées, 10 faibles) et **10 alertes serveur** (2 élevées,
5 modérées, 3 faibles). Aucune alerte ouverte dans `hbb_common` à ce contrôle.
Une absence d'alertes avant activation n'était pas une preuve d'absence de
vulnérabilité. Les comptes peuvent évoluer après analyse de GitHub.

## Qualification initiale, pas une résolution

| Périmètre | Constat | Travail technique nécessaire |
| --- | --- | --- |
| Client, OpenSSL | 8 alertes ; `Cargo.lock` contient `openssl 0.10.72`. La dépendance directe est ciblée Linux/Android. Les correctifs recensés vont jusqu'à 0.10.80. | Mettre à jour, examiner les appels atteignables et reconstruire/tester les plateformes concernées. Ne pas déduire une compromission Windows de cette seule entrée. |
| Client, FUSE | 2 signalements du même problème `fuser < 0.16.0`, dans le manifeste et le lockfile. Fonction Linux `unix-file-copy-paste`. | Migration 0.16+ et recette réelle du transfert de fichiers/FUSE ; ne pas considérer deux alertes comme deux vulnérabilités distinctes. |
| Client, lockfile secondaire | 10 signalements dans `libs/virtual_display/Cargo.lock` ; le composant appartient désormais au workspace racine. | Vérifier le lockfile effectivement utilisé par chaque build avant de conclure à une exposition. Un fichier secondaire ancien n'est pas une preuve du contenu d'un EXE. |
| Client, autres | `rand`, `rpassword`, `glib`, `atty` dans le lockfile principal. Certaines migrations ne sont pas de simples mises à jour correctives ; `atty` n'annonce pas de version corrigée. | Qualifier les plateformes et usages, corriger/remplacer avec tests. |
| Serveur, UI historique | 6 alertes Vite (développement) et 3 dépendances dans `ui/Cargo.lock`. L'image RelaisDesk fournie construit `hbbs`/`hbbr`, pas cette UI. | Maintenir l'UI si elle est utilisée, ne pas exposer son serveur de développement ; ne pas attribuer automatiquement ses 9 alertes au service Oracle. |
| Serveur, binaire principal | 1 alerte faible `atty`, sans version corrigée indiquée. | Examiner les conditions d'exploitation et remplacer la chaîne de dépendances concernée avec tests CLI. |

Les correctifs de cette livraison portent sur le juridique, les notices,
la publication des sources et les réglages GitHub. **Ils ne corrigent pas
ces dépendances Rust/UI et ne reconstruisent pas le moteur RustDesk.**
Les exécutables déjà distribués ne sont pas corrigés par une mise à jour
documentaire ou l'activation d'un robot GitHub.

Les propositions Dependabot sont activées, sans fusion automatique. Aucune
alerte n'a été masquée ou classée comme faux positif faute de preuve.
Prévoir une itération technique dédiée : mises à jour compatibles, adaptations
nécessaires, tests Windows/Linux, nouveaux binaires, nouveaux commits de
provenance et nouveau manifeste. Réévaluer alors les pièces de cryptologie
uniquement si les caractéristiques déclarées ont changé ; ne pas confondre
un correctif logiciel et une notification d'incident avéré.

## Sources contrôlables

- Alertes client : https://github.com/JuLeXoGame/relaisdesk/security/dependabot
- Alertes serveur : https://github.com/JuLeXoGame/rustdesk-server/security/dependabot
- OpenSSL : https://github.com/advisories/GHSA-phqj-4mhp-q6mq
- FUSE : https://github.com/advisories/GHSA-cvmj-47v9-35m9
- `atty` : https://github.com/advisories/GHSA-g98v-hv3f-hcfr

Les pages de sécurité détaillées peuvent nécessiter une connexion de
mainteneur. Ce document ne contient aucune preuve d'exploitation active.
