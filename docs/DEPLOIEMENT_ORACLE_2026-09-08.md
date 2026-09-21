# Mise à jour Oracle - 8 septembre 2026

**Terminée et vérifiée le 8 septembre 2026 à 00:38 (Europe/Paris).**
Contrôle final UTC : `2026-09-07T22:38:43Z`.

## Nouvelle API : activée et vérifiée

- Service : `relaisdesk-api.service`, compte `relaisdesk`.
- Binaire actif : `/opt/relaisdesk/api/api`.
- SHA-256 du binaire réellement exécuté : `e04410a896c40047aa35c8156e2c531dd157263d9cf3969a2b49ece82f945ba2`.
- Santé HTTPS : `healthy`, base `connected`.
- Écoute : `127.0.0.1:8443`, derrière le Nginx existant.
- Les 13 fichiers OVH ont été vérifiés en ligne et correspondent aux corrections
  préparées (normalisation des fins de ligne uniquement). Aucune écriture OVH
  n'a été faite dans cette opération Oracle.
- Les futures commandes utilisent les CGV version `2026-09-07`. L'exploitant
  confirme qu'il n'existe pas de commande déjà acceptée à reprendre ; aucune
  opération de reprise d'anciennes commandes n'a donc été exécutée.
- Aucun changement de `.env`, de clé, de DNS, de certificat ou de Nginx.
  `B2C_SALES_ENABLED=false` conservé. Conteneurs RustDesk inchangés.

L'API a été activée seulement après la mise à jour constatée du site OVH.
Sa santé et l'empreinte de `/proc/<PID>/exe` ont été contrôlées après redémarrage.
Aucune commande commerciale, transaction Stripe ni campagne d'email de test
n'a été créée pour ces vérifications.

## Sauvegarde avant activation

Répertoire privé (root) :
`/opt/relaisdesk/deployments/legal-20260908-IXEPGqyW/rollback-api/`

- `api` : ancien binaire, empreinte `52bdde24eab2104feee2c46ed1b738d0659a0fd9b977f5bfcde4ceb114b2d10f`.
- `licences-before-api.db` : sauvegarde SQLite cohérente par `.backup`,
  intégrité `ok`, droits `0600`. Ce n'est pas une simple copie d'une base WAL
  ouverte et ce n'est pas une base de développement.
- `licences-migration-test.db` : copie distincte utilisée pour le contrôle
  de migration/intégrité avec `relaisdesk-dbcheck` ; contrôle réussi.

La base active reste `/data/relaisdesk/licences.db`. Aucune sauvegarde n'a été
restaurée sur la base active. En cas de problème ultérieur, ne pas écraser
cette base par un ancien instantané sans analyser les écritures intervenues.

## Paquet de préparation et sources

Répertoire serveur :
`/opt/relaisdesk/deployments/legal-20260908-IXEPGqyW/`

Le sous-dossier `oracle/` contient le binaire neuf, un vérificateur de manifeste
en lecture seule et `api-database-sources.tar.gz` (87 fichiers source nécessaires
à la reconstruction de cette API : Go, modules, SQL et archives contractuelles).
Le paquet ne contient pas la clé privée de signature, le `.env`, la base active,
la clé SSH ou les documents ANSSI. Les anciens fichiers de `/srv/relaisdesk-src`
ne sont pas écrasés : utiliser la source de ce paquet pour reconstruire la
version juridique actuelle.

## Téléchargements

Les téléchargements sont déployés dans `/opt/relaisdesk/downloads`, avec le
manifeste signé **`1.0.0-legal.20260907`**. Deux installateurs Windows ont été
remplacés ; les quatre portables/paquets Linux restent inchangés. Le nouveau
`SHA256SUMS.txt` et le manifeste ont été remplacés dans la même bascule.

- SHA-256 du manifeste : `e2e0bb9d202d9c176f79e70a93b4f817e0b85013e420833878edf35c54a08bca`.
- Signature Ed25519 et empreintes des 7 artefacts : vérifiées sur Oracle,
  avant puis après bascule, avec la clé publique existante.
- Manifeste public vérifié depuis Internet : version, signature, noms,
  tailles et empreintes identiques au paquet préparé.
- Les deux nouveaux installateurs sont accessibles en HTTPS ; requêtes
  partielles vérifiées (`206`, 32 octets). Les empreintes intégrales ont été
  vérifiées directement sur le serveur par le vérificateur de manifeste.
- Après la dernière bascule : API `active/running`, zéro redémarrage automatique
  en erreur ; SHA-256 du processus identique au nouveau binaire sur disque.
- `hbbs` et `hbbr` conservent leurs conteneurs et leur fonctionnement continu.

Les téléchargements précédents et une sauvegarde SQLite cohérente supplémentaire
sont conservés sous :
`/opt/relaisdesk/deployments/legal-20260908-IXEPGqyW/rollback-downloads/`

Les deux répertoires de sauvegarde sont `root:root`, mode `0700`. Les instantanés
SQLite sont en mode `0600`. Les copies temporaires de transfert sont conservées
dans `/home/ubuntu/relaisdesk-legal-20260908-375cAJTK/` ; aucun secret n'y figure.

## Retour arrière

Ne pas réexécuter les scripts de mise à jour : leurs garde-fous empêchent une
seconde exécution sur un état déjà modifié. En cas de problème, vérifier l'état
réel et utiliser les sauvegardes privées ci-dessus pendant une courte maintenance.

- Revenir à l'ancienne API nécessite de coordonner les fichiers OVH : l'ancienne
  API attendait les CGV `2026-08-25`, le site actuel envoie `2026-09-07`.
- Revenir aux anciens téléchargements nécessite de restaurer **le répertoire
  complet**, manifeste compris, pas uniquement un EXE.
- Ne pas restaurer automatiquement la base SQLite : les opérations de cette
  mise à jour n'ont pas remplacé la base active et de nouvelles écritures peuvent
  s'y produire après les sauvegardes.
- Les fichiers d'environnement, clés, certificats, Nginx et conteneurs RustDesk
  n'ayant pas été modifiés, ils ne nécessitent aucun retour arrière pour ce lot.

Les 35 signalements Dependabot recensés séparément ne sont pas corrigés par
cette livraison juridique et restent un chantier technique distinct.
