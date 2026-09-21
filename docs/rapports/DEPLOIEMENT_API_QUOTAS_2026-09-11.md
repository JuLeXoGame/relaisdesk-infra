# Déploiement de l'API et des quotas — 11 septembre 2026

## Résultat

L'API Oracle a été mise à jour à la demande de l'exploitant, directement en production. Le nouveau processus a démarré le 10 septembre à 22:30:14 UTC, soit le 11 septembre à 00:30:14 à Paris.

- Service : `relaisdesk-api.service`, actif, PID 265641, aucun redémarrage automatique constaté après la bascule.
- Binaire installé : `/opt/relaisdesk/api/api`, Linux amd64 statique, compilé avec Go 1.26.6 sans mise à jour de dépendances.
- SHA-256 : `b693543cf7e39cff8034ebcb3121be25071c6ded99e34f527a0f518547961a60`.
- Base réellement utilisée : `/data/relaisdesk/licences.db` ; environnement de facturation conservé en `live`.
- Écoute conservée sur `127.0.0.1:8443`, derrière le Nginx existant.
- Aucun changement des fichiers d'environnement, du service systemd, du VPN WireGuard, des conteneurs hbbs/hbbr, des mises à jour système ni des binaires clients disponibles au téléchargement.

Les quotas sont désormais servis par l'API publique : Starter 500, Pro 1 000, Ultra 2 000 pour 10 techniciens, puis +5 appareils par technicien supplémentaire. Le dernier palier de 500 techniciens est arrondi à 4 500 appareils. Les prix restent 24,90 €/mois pour Starter, 110 €/mois pour Pro et 199 €/mois à la base d'Ultra.

## Contrôles réalisés

1. Tests des modules Go `database` et `api`, puis contrôles des tarifs et des contrats web : succès.
2. Vérification des empreintes après transfert SSH et vérification du format ELF amd64.
3. Migration et intégrité testées sur une copie privée de la base réelle avant tout arrêt du service.
4. Copie fraîche de la base via l'API de sauvegarde SQLite après arrêt des écritures, puis remplacement atomique du seul exécutable API.
5. Contrôle du SHA-256 du processus réellement démarré, pas seulement du fichier installé.
6. Contrôle HTTPS local et public de `/api/v1/health` : base connectée, état sain.
7. Contrôle HTTPS public de `/api/v1/public/pricing` : plafonds 500 / 1 000 / 2 000–4 500, progression et arrondi corrects, prix inchangés.
8. Contrôle de `/api/v1/public/trials` : essai de 30 jours toujours activé, CGV `2026-09-10`, conditions d'essai `2026-09-10-fleet-v1`.
9. Routes de parc client et technicien : accès anonyme refusé avec HTTP 401.
10. `PRAGMA integrity_check` : `ok` ; aucune erreur de clé étrangère.
11. Empreintes des configurations API/systemd/WireGuard inchangées ; identifiants et dates de démarrage des conteneurs RustDesk inchangés.

La base contenait avant déploiement une licence active de capacité 5 et une licence révoquée, aucune commande et aucun essai. La table de parc n'était pas encore présente ; elle a été créée par la migration et reste vide. Aucun faux appareil, achat ou essai n'a été créé en production pour les tests. Le refus du 4 501e appareil a été testé dans les tests automatisés, pas en remplissant le parc réel.

## Retour arrière

Une copie ponctuelle de sécurité, réservée à cette opération et non une nouvelle sauvegarde planifiée, est conservée sur Oracle :

`/opt/relaisdesk/deployments/quotas-20260911-K97gRv13/`

- `api.previous` : ancien exécutable, root:root, 0700 ; SHA-256 `fcb9cb1617266ee9c2b29376d77641119168391e932c5eaf4d33d96fa657b694`.
- `licences.before.db` : copie SQLite cohérente avant bascule, root:root, 0600.
- `api.candidate` : exécutable de cette livraison ; `relaisdesk-dbcheck` : outil ayant validé la migration.
- `preflight/licences.db` : copie de contrôle migrée, distincte de la base de production.

Le script `scripts/deploy_api_quota_update.sh` restaure l'ancien exécutable si un contrôle immédiat de bascule échoue. Il ne rembobine jamais automatiquement la base : une restauration tardive risquerait de perdre de nouvelles écritures ou des événements Stripe. Aucun retour arrière n'a été nécessaire.

Cette copie sur le même VPS sert au retour arrière de déploiement ; elle ne remplace pas une sauvegarde hors site. La connexion Google Drive n'a pas été modifiée par cette opération.

## OVH et limites de validation

Aucune publication du site OVH n'a été effectuée, aucun accès de déploiement OVH n'étant disponible. Avant même la bascule, les requêtes de contrôle depuis Windows renvoyaient 403 pour `https://relaisdesk.fr/` et 404 pour les fichiers web testés ; la tentative HTTPS depuis Oracle était réinitialisée. Ces observations ne prouvent pas une indisponibilité pour tous les visiteurs, mais empêchent de confirmer l'état des pages publiées.

Le parcours d'achat complet et l'affichage public des nouvelles conditions ne sont donc pas validés. Vérifier l'accès OVH et transférer, si nécessaire, les fichiers du dossier de référence `relaisdesk/` en conservant leur arborescence : `index.html`, `app.js`, `i18n.js`, `cgv.html`, `client/index.html`, `client/app.js`, `essai/index.html`, `essai/app.js`, `essai/conditions.html`. Une ancienne page de commande utilisant les CGV du 9 septembre sera refusée par la nouvelle API ; recharger la page après publication.

Le paramétrage B2C déjà actif a été conservé. L'API signale toujours `mediation_pending: true` : cette mise à jour technique ne régularise ni ne valide la situation relative au médiateur de la consommation.

Aucun test de charge à plusieurs milliers d'appareils ni paiement réel n'a été réalisé. Aucune modification DNS, GitHub ou publication de nouveaux binaires clients n'a été faite.
