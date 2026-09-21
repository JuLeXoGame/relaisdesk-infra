# Rangement du projet — 9 septembre 2026

## Résultat

**41 déplacements vérifiés**, portant sur 5 385 fichiers (882,60 Mio réorganisés). Aucun fichier supprimé définitivement, aucune publication GitHub/OVH et aucune intervention Oracle. Ce rangement n'a pas libéré ces 882,60 Mio : les archives restent sur le disque pour préserver les sauvegardes et l'historique.

Avant chaque déplacement, les chemins source et destination ont été vérifiés comme appartenant au projet, sans jonction ni lien traversé, et sans écrasement d'une destination existante. Le contenu de chaque fichier a été comparé par SHA-256 avant/après le déplacement. Aucun contenu ni empreinte de secret n'est inscrit dans le journal.

La racine conserve les sources, dossiers techniques et quatre fichiers usuels visibles : `README.md`, `CHANGELOG.md`, `Makefile`, `.gitignore`. Le fichier système caché Windows `desktop.ini` a été laissé intact.

## Nouvelle organisation

| Contenu | Emplacement actuel |
| --- | --- |
| Sources et outils | `api/`, `database/`, `installer/`, `rustdesk/`, `rustdesk-server/`, `relaisdesk/`, `dashboard_admin/`, `keygen/`, `scripts/`, `tests/` — inchangés |
| Guides actuels | [Index de documentation](README.md) |
| Notes anciennement à la racine | [docs/notes](notes/README.md) |
| Audits d'août anciennement à la racine | [docs/audits](audits/README.md) |
| Ancien dossier rapports | `docs/rapports/` |
| Modèles d'identifiants sans secrets | [docs/exemples](exemples/README.md) |
| Clé Oracle et deux fichiers d'identifiants privés | `.secrets/` |
| Dernière livraison API/site | `output/essai-production-2026-09-09/` |
| Binaires sécurité à tester | `output/security-2026-09-08/` — inchangé |
| PDF rectificatifs | `output/pdf/` — inchangé |
| Nouvelles captures de tests navigateur | `output/tests/parcours-essai/` |
| Anciennes livraisons | `archives/livraisons/` |
| Ancien proxy | `archives/obsolete-proxy-artifacts/` |
| Ancien scratch, tmp et exécutables de test | `archives/nettoyage-2026-09-09/temporaires/` |
| Copies identiques de la livraison web | `archives/nettoyage-2026-09-09/doublons-site-ovh/` |
| Dernier audit de maintenance | `update/20260908-205353-a8ce1b1e/` — inchangé |
| Trois anciens audits de maintenance | `update/archives/` |

Les documents PDF signés, justificatifs, `.env`, bases de données, exécutables embarqués, caches de compilation et outils locaux ont été laissés à leur emplacement. Les deux dépôts Git des forks sont restés sans modification locale.

## Point important : clé Oracle

Ancien chemin : `oracle private.ppk` à la racine.

Nouveau chemin : `.secrets/oracle private.ppk`, soit `C:\Users\Administrator\Documents\Projets\projet\.secrets\oracle private.ppk`.

Adapter uniquement le chemin du fichier dans les éventuelles sessions PuTTY/WinSCP enregistrées. La clé n'a pas été modifiée, régénérée ni envoyée ailleurs. Les recherches dans les scripts actifs n'ont pas révélé de connexion utilisant son ancien chemin. Les références conservées dans l'historique ne sont pas des configurations à jour.

Le dossier `.secrets/`, les archives et les livraisons sont désormais explicitement exclus par le `.gitignore` racine. Les tests confirment que les copies de maintenance n'intègrent ni `.secrets/` ni `archives/`. Cela ne chiffre pas les fichiers et ne modifie pas les permissions des comptes Windows.

## Livraisons : éviter les confusions

Le dossier de production conserve une seule arborescence décompressée du site : `site-ovh/`, avec son ZIP de référence. Les 16 fichiers qui étaient dupliqués à côté ont été archivés après comparaison d'intégrité. Les archives ZIP, exécutables API, contrôleurs SQLite et manifestes n'ont pas été reconstruits.

Les anciens déploiements juridiques, de prix et d'essai v1 sont rangés dans `archives/livraisons/`. Ne pas utiliser leurs scripts pour revenir à une ancienne API sur une base qui contient de nouveaux abonnements.

Les chemins de la documentation active ont été ajustés. Le test navigateur écrit désormais ses captures dans `output/tests/parcours-essai/`, sans recréer une ancienne livraison. Les textes, scripts et métadonnées embarqués dans les archives historiques restent des instantanés de leur époque.

## Traçabilité et récupération

Contrôles finaux : 88 liens locaux valides, empreintes des livraisons conservées, doublons archivés identiques et test navigateur isolé réussi dans son nouveau dossier. Les tests de maintenance ont donné 31 réussites et un test de création de lien symbolique ignoré faute de droits Windows. Les contrôles des contrats et de la redirection OVH passent ; ils ne déploient rien.

- [Journal des 41 déplacements](../archives/nettoyage-2026-09-09/deplacements.json)
- [Plan exact ancien/nouveau chemin](../archives/nettoyage-2026-09-09/plan.json)
- Script ponctuel conservé : `archives/nettoyage-2026-09-09/ranger-projet.ps1` ; ne pas relancer un rangement déjà terminé.

Pour récupérer un fichier, le retrouver à la destination indiquée dans le journal. Un retour manuel à son ancien emplacement est possible après vérification qu'aucun nouveau fichier ne l'occupe. Ne pas écraser des changements plus récents ni déplacer des dossiers entiers sans contrôler leurs chemins ; les liens de documentation seraient aussi à réadapter.

Pour garder le dossier propre, utiliser `docs/notes/` pour les nouvelles notes, `output/tests/` pour les validations, `output/` pour les livraisons identifiées et `.secrets/` pour les identifiants locaux. Le script de maintenance continue d'écrire sous `update/`.
