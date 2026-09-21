# Déploiement des correctifs de sécurité Oracle — 12 septembre 2026

Terminé à la demande de l'exploitant. Démarrage de la nouvelle API le **12 septembre 2026 à 13:36:27, heure de Paris** (11:36:27 UTC). Contrôle final de stabilité à 11:37:52 UTC.

## Périmètre effectivement publié

- Nouvelle API : `/opt/relaisdesk/api/api`, Linux amd64 statique, Go 1.26.6, issue des sources locales corrigées.
- Migration d'authentification `20260912-auth-v1` appliquée à `/data/relaisdesk/licences.db`.
- Une seule ligne ajoutée à `/etc/nginx/sites-available/relaisdesk-api` : `proxy_set_header CF-Connecting-IP "";`. Les autres réglages, notamment les écoutes IPv4/IPv6, ont été conservés à l'identique.
- Nginx rechargé après vérification syntaxique ; aucune création de bloc pour le site OVH.

SHA-256 du nouveau binaire installé **et du processus réellement exécuté** :

`ba90f5dde481aaba1e007fa597dc1ee4cc220e5db334bed7c147b5af1c3ae7cd`

Ancien binaire : `937b05bd75b3e2b1acd7366162ee7dfe949040df7d956eccb09b31462d7e5cce`.

Le champ public `version` du point de santé reste `1.0.0` ; il ne permet pas de distinguer cette livraison. L'empreinte du processus est la référence vérifiée.

## Contrôles réalisés

1. Suites Go complètes `api` et `database` réussies avant compilation Linux.
2. Empreintes des fichiers transférés et format ELF amd64 statique vérifiés.
3. Neuf tests de non-régression d'authentification exécutés avec succès directement sur Oracle, sous l'utilisateur `ubuntu`, avec des bases SQLite temporaires synthétiques. Aucun compte de test ajouté en production, aucun paiement ni e-mail déclenché par ces tests.
4. Migration testée sur une copie privée de la base réelle : intégrité et clés étrangères valides, licences/appareils/commandes/comptes préservés, anciennes sessions et liens invalidés sur cette copie.
5. Sauvegarde SQLite fraîche après arrêt de l'API, puis remplacement atomique de l'exécutable.
6. API saine en HTTPS local et public, base connectée. Processus PID `296129`, `active/running`, `NRestarts=0` au contrôle final ; écoute toujours sur `127.0.0.1:8443`.
7. Réponses des offres, quotas, essais et versions contractuelles comparées avant/après : identiques. Pro toujours 110 €/mois, plafonds Starter/Pro/Ultra 500/1 000/2 000–4 500, essai 30 jours. Conditions `2026-09-11` et `2026-09-11-fleet-v2` conservées.
8. Routes privées client, technicien et administrateur : accès anonyme refusé en HTTPS public avec HTTP 401.
9. Intégrité SQLite `ok`, aucune erreur de clé étrangère. Marqueur de migration présent une fois. Comparaison avec la sauvegarde : aucune ancienne session administrateur/client/technicien ni ancien lien e-mail encore présent après déploiement.
10. Données métier avant/après : 2 licences, 1 appareil, 0 commande et 7 comptes, sans variation constatée.
11. Empreintes des fichiers d'environnement, unité systemd, configuration WireGuard et de tous les fichiers directement présents dans les téléchargements inchangées. Conteneurs `hbbs`/`hbbr` inchangés, sans redémarrage. Service WireGuard toujours actif, même date de démarrage.

Les tests automatisés ne remplacent pas une recette interactive de chaque compte ni un audit exhaustif.

## Sauvegardes et retour arrière

Répertoire Oracle :

`/opt/relaisdesk/deployments/security-20260912-kEKisJS6/`

- `api.previous` : ancien exécutable, root:root, 0700.
- `nginx-api.previous` : ancienne configuration Nginx, root:root, 0600.
- `licences.before.db` : copie SQLite cohérente juste avant bascule, root:root, 0600.
- `api.candidate`, `nginx-api.candidate`, `relaisdesk-dbcheck` : fichiers de cette livraison.
- `preflight/licences.db` : copie distincte utilisée pour tester la migration ; aucun retour de cette copie vers la production.

Le script local `scripts/deploy_api_security_update.sh` sépare préparation et bascule, vérifie l'identité de l'ancienne API/configuration et utilise un verrou de déploiement. Son retour arrière automatique rétablit l'exécutable et Nginx si un contrôle immédiat échoue, **sans restaurer automatiquement la base**. Aucun retour arrière n'a été nécessaire.

En cas de retour arrière ultérieur, ne pas écraser la base par la sauvegarde sans analyser les nouvelles écritures. Restaurer l'ancien binaire réintroduirait les faiblesses d'authentification corrigées. Les anciennes sessions supprimées ne sont pas recréées par un simple retour de binaire.

Cette copie ponctuelle sur le même VPS permet un retour arrière de déploiement ; elle ne remplace pas une sauvegarde hors site. Aucune configuration de sauvegarde Google Drive ni tâche planifiée n'a été modifiée.

Les fichiers de transfert, sans secrets ni base réelle, restent dans `/home/ubuntu/relaisdesk-api-security-20260912-tNwBu28s/`. Les anciennes sources de `/srv/relaisdesk-src` n'ont pas été écrasées ; pour reconstruire les correctifs, utiliser les modules locaux corrigés `api/` et `database/`, pas une ancienne copie serveur.

## OVH et programmes clients

Avant la bascule, `https://relaisdesk.fr/client/index.html` et `client/app.js` ont été comparés aux fichiers locaux corrigés : contenu identique après normalisation des fins de ligne. Aucun transfert OVH n'a été fait par cette intervention et aucune nouvelle archive web n'a été créée.

Les exécutables clients et leur manifeste restent inchangés. Les corrections propres au stockage du Technicien et au service permanent Windows nécessitent encore la reconstruction et les essais des nouveaux programmes avant leur publication. Aucun envoi GitHub n'a été réalisé.

Les utilisateurs doivent se reconnecter après l'invalidation unique des sessions. Leurs licences et appareils restent enregistrés.
