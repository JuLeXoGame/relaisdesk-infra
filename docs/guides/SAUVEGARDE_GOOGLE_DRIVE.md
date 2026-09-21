# Sauvegarde RelaisDesk vers Google Drive

## Périmètre et état

Cette sauvegarde concerne uniquement `/data/relaisdesk/licences.db` : clients,
licences, commandes, essais, abonnements, informations des factures, interventions,
appareils et autres tables de la base. Les PDF, `.env`, clés externes à la base,
données hbbs/hbbr, configurations système et fichiers OVH ne sont pas inclus.
Ce n'est pas une image complète du VPS.

La préparation du 10 septembre 2026 ne vaut pas activation : la connexion Google,
une première sauvegarde distante vérifiée et la conservation hors serveur de la
clé de déchiffrement doivent être réalisées avant de considérer ce dispositif
comme opérationnel. Aucun réglage WireGuard, pare-feu, mise à jour système,
redémarrage API ou déploiement de client n'est nécessaire.

Fonctionnement :

1. Instantané SQLite cohérent, chiffrement AES-256-GCM et vérification locale par
   l'exécutable `relaisdesk-backup` existant.
2. Copie des seules archives `relaisdesk-AAAAMMJJTHHMMSSZ.db.aesgcm` vers
   `relaisdesk-drive:RelaisDesk-Backups/julexogame` ; pas de fichier en clair ou de
   clé envoyée. Les copies locales en attente d'une tentative précédente sont
   également envoyées.
3. Relecture complète de la dernière archive depuis Drive, comparaison SHA-256,
   déchiffrement temporaire et vérification SQLite. Cela ne remplace pas une
   recette complète de reprise de l'application.
4. Seulement après succès : conservation locale de 30 jours et actualisation de
   `/var/backups/relaisdesk/google-drive-last-success.json`.

Il n'y a **aucune suppression automatique sur Google Drive**. La commande `copy`
avec `--immutable` refuse les remplacements ; elle n'est pas un verrou Google
anti-effacement : le titulaire du compte ou un serveur compromis disposant du
jeton peut encore supprimer des fichiers. Le quota Drive doit être surveillé.
En cas d'échec, le service échoue et garde les archives locales, mais il n'envoie
pas d'alerte par email. Il faudra consulter son état/journal.

## 1. Compte Google et autorisation

Utiliser un compte maîtrisé par l'entreprise, avec double authentification. Ne
pas partager publiquement le dossier de sauvegarde. Les archives restent des
données de l'entreprise même lorsqu'elles sont chiffrées.

Les instructions officielles sont disponibles sur
[rclone Google Drive](https://rclone.org/drive/) et
[la configuration sans navigateur](https://rclone.org/remote_setup/).

Créer son propre client OAuth : le client partagé rclone est annoncé en retrait
en 2026 dans sa documentation. Dans la console Google Cloud :

1. Créer/sélectionner un projet dédié, par exemple `RelaisDesk Backups`.
2. Activer **Google Drive API**.
3. Configurer **Google Auth Platform** : nom de l'application et contacts de
   l'exploitant. Audience interne seulement si le compte Workspace le permet ;
   sinon externe. Ajouter son compte comme utilisateur de test si nécessaire.
4. Dans l'accès aux données, demander uniquement
   `https://www.googleapis.com/auth/drive.file`. Ne pas demander l'accès à tout
   Drive, aux documents Google ou à Gmail.
5. Créer un client OAuth de type **Application de bureau**. Conserver son ID et
   son secret de client dans un emplacement privé, pas dans la conversation.
6. Pour une audience externe, ne pas laisser durablement l'application en mode
   **Test** : Google limite alors généralement la durée du jeton de renouvellement
   à sept jours pour cet accès. Terminer la configuration nécessaire dans Google
   avant d'activer la sauvegarde quotidienne. Ne pas contourner les demandes de
   validation de Google.

Le scope `drive.file` limite l'accès aux fichiers créés/autorisés à cette
application. Laisser rclone créer le dossier de sauvegarde, plutôt que de le
créer manuellement sur Drive. Une restriction de chemin seule n'est pas une
restriction des droits OAuth.

## 2. Connexion depuis Windows, sans copier les jetons dans une discussion

Télécharger rclone depuis [le site officiel](https://rclone.org/downloads/),
vérifier le SHA-256 avec le fichier `SHA256SUMS` de la même version, puis extraire
le programme dans un dossier privé local. Utiliser idéalement la même version
que sur le serveur. Cela ne demande aucune mise à jour des autres programmes.

Sur le poste Windows du projet, la version portable **v1.75.1** a déjà été
téléchargée et son archive comparée au SHA-256 officiel. Depuis PowerShell,
lancer :

```powershell
& "C:\Users\Administrator\Documents\Projets\projet\.tools\rclone-v1.75.1\rclone-v1.75.1-windows-amd64\rclone.exe" config --config "C:\Users\Administrator\Documents\Projets\projet\.secrets\rclone.conf"
```

Réponses à donner dans l'assistant :

- Nouveau remote : `relaisdesk-drive`.
- Stockage : `drive` (Google Drive).
- `client_id` et `client_secret` : ceux créés dans la console Google.
- `scope` : **drive.file**, accès aux seuls fichiers créés par l'application.
- Compte de service : vide ; configuration avancée : non.
- Autorisation avec navigateur : oui. Choisir le compte de sauvegarde, vérifier
  les droits demandés et autoriser soi-même dans Google.
- Shared Drive : non pour un Drive personnel standard.
- Enregistrer le remote puis quitter. Ne pas publier la sortie de l'assistant :
  elle peut contenir des jetons. Le fichier rclone.conf est un secret.

Transférer ce fichier sur Oracle par SFTP/SCP, sans en afficher le contenu. Le
chemin final est `/etc/relaisdesk/rclone/rclone.conf`, propriétaire
`relaisdesk:relaisdesk`, mode `0600`, dans un dossier `0700` de même propriétaire.
Le dossier doit être inscriptible par le service pour le renouvellement des
jetons OAuth. Ne pas mettre ce fichier dans `relaisdesk/` (OVH) ou dans Git.

## 3. Préparation serveur (sans activation)

Le 10 septembre 2026, ces nouveaux fichiers ont été installés sur Oracle avec
rclone **v1.75.1**. Le minuteur Google Drive reste **désactivé et inactif** ;
l'ancien minuteur n'a pas été modifié. Les 13 tests hors connexion ont réussi
sous Ubuntu. Un test bout en bout utilisant les vrais exécutables et une base
fictive a réussi avec le stockage local de rclone, **pas encore avec Google**.
L'API n'a pas été redémarrée et aucune base de production n'a été exportée.

Fichiers à installer depuis les sources :

- `scripts/backup_google_drive.py` →
  `/opt/relaisdesk/backup-tools/backup_google_drive.py` (`root:root`, `0755`).
- Binaire rclone officiel vérifié → `/opt/relaisdesk/backup-tools/rclone`
  (`root:root`, `0755`). Pas d'installation globale ni d'`apt upgrade` requis.
- `api/relaisdesk-backup-google-drive.service` et `.timer` →
  `/etc/systemd/system/` (`root:root`, `0644`).
- `/var/backups/relaisdesk` : `relaisdesk:relaisdesk`, `0700`.

L'unité Google Drive est indépendante de l'ancien minuteur local. Elle utilise
les chemins de production ci-dessus explicitement, pas `backup.env`. Si les
chemins changent, adapter son ExecStart avec les options de `--help`.

## 4. Premier test et activation — après autorisation Google uniquement

Sauvegarder séparément `/etc/relaisdesk/backup.key` dans un coffre hors ligne
ou un gestionnaire sécurisé. Ce fichier n'est **jamais envoyé dans Drive par le
script**. Sa perte rend les archives indéchiffrables ; ne pas le régénérer pour
essayer de restaurer les anciennes copies.

```bash
sudo -u relaisdesk python3 -B /opt/relaisdesk/backup-tools/backup_google_drive.py --check-config
sudo systemd-analyze verify /etc/systemd/system/relaisdesk-backup-google-drive.service
sudo systemctl daemon-reload
sudo systemctl start relaisdesk-backup-google-drive.service
sudo systemctl status relaisdesk-backup-google-drive.service --no-pager -l
sudo journalctl -u relaisdesk-backup-google-drive.service -n 20 --no-pager
sudo cat /var/backups/relaisdesk/google-drive-last-success.json
```

`--check-config` est un contrôle local, **pas une preuve d'accès à Google**.
Après un oneshot réussi, `inactive (dead)` est normal : rechercher `status=0/SUCCESS`
et un fichier de succès récent. Une date ancienne n'est pas une preuve de succès
de la dernière tentative. Vérifier aussi la présence de l'archive dans Drive.

Seulement après ce succès et la mise en sécurité de la clé :

```bash
sudo systemctl disable --now relaisdesk-backup.timer
sudo systemctl enable --now relaisdesk-backup-google-drive.timer
sudo systemctl list-timers relaisdesk-backup-google-drive.timer --no-pager
```

La désactivation concerne uniquement l'ancien minuteur de sauvegarde locale,
pour éviter deux créateurs simultanés. Ne pas arrêter l'API, Docker ou WireGuard.
L'horaire prévu est 03:15 avec jusqu'à 30 minutes de décalage, dans le fuseau du
serveur (UTC sur le serveur audité). Une exécution persistante peut avoir lieu
rapidement lors de l'activation après une échéance manquée.

Un redéploiement générique de l'API peut réactiver l'ancien minuteur : vérifier
alors que seul le minuteur Google Drive reste actif.

## 5. Récupérer et vérifier une sauvegarde

Télécharger depuis Drive l'archive choisie, conserver la clé séparément, puis :

```bash
/opt/relaisdesk/api/relaisdesk-backup verify -file /chemin/archive.db.aesgcm -key-file /chemin/backup.key
/opt/relaisdesk/api/relaisdesk-backup restore -file /chemin/archive.db.aesgcm -key-file /chemin/backup.key -out /chemin/licences-restaurees.db
```

Le fichier de sortie doit être nouveau. Ne pas remplacer la base active pendant
que l'API fonctionne. La remise en service effective constitue une opération
distincte, à préparer avec les configurations et clés externes à la base.

Références : [permissions Drive](https://developers.google.com/workspace/drive/api/guides/api-specific-auth),
[expiration OAuth](https://developers.google.com/identity/protocols/oauth2#expiration),
[copie sans synchronisation](https://rclone.org/commands/rclone_copy/).
