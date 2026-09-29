# Administration

## Grille tarifaire & Calculateur

```bash
# Afficher toute la grille tarifaire (Starter, Pro, Ultra)
./keygen pricing

# Calculer le tarif mensuel Ultra pour un nombre spécifique de techniciens (ex: 40 techs)
./keygen pricing --technicians 40
```

## Generer une licence

```bash
# Plan Starter (10€/mois, 1 technicien, viewers illimités)
./keygen generate --email client@example.com --plan starter

# Plan Pro (20€/mois, jusqu'à 10 techniciens, viewers illimités)
./keygen generate --email client@example.com --plan pro

# Plan Ultra (ex: 20 techniciens = 30,00€/mois, viewers illimités)
./keygen generate --email client@example.com --plan ultra --technicians 20

# Licence personnalisée
./keygen generate --email client@example.com --days 30 --max_connections 5 --notes "Sur mesure"
```

## Lister

```bash
./keygen list
./keygen list --status active
./keygen list --email client@example.com
```

## Revoquer

```bash
./keygen revoke --id MP-XXXX-XXXX-XXXX --reason "Remboursement"
```

## Prolonger

```bash
./keygen extend --id MP-XXXX-XXXX-XXXX --days 30
```

## Sauvegarde DB et factures

Ne jamais copier directement `licences.db` pendant que l'API fonctionne : la
base utilise le mode WAL et une copie brute peut être incohérente. Le timer
quotidien crée à la place un instantané SQLite cohérent, contrôle son intégrité,
le chiffre en AES-256-GCM, le déchiffre dans un fichier temporaire et vérifie
qu'il est restaurable avant de valider la sauvegarde.

La même exécution scelle aussi le dossier des factures
(`/data/relaisdesk/invoices`, conservation fiscale de dix ans) dans une archive
`relaisdesk-AAAAMMJJTHHMMSSZ.invoices.aesgcm` déterministe (tar trié, chemins
relatifs, liens refusés), chiffrée avec la même clé et contrôlée à blanc
(déchiffrement + relecture intégrale + comparaison d'empreinte avec l'original).
Le `manifest.json` liste les deux archives ; la rétention et le miroir hors site
s'appliquent aux deux. Si le dossier des factures est inaccessible, la
sauvegarde échoue immédiatement sans rien valider.

```bash
systemctl status relaisdesk-backup.timer
systemctl list-timers relaisdesk-backup.timer

# Lancer une sauvegarde immédiatement et lire son résultat.
sudo systemctl start relaisdesk-backup.service
journalctl -u relaisdesk-backup.service -n 50 --no-pager

# Vérifier manuellement une sauvegarde existante.
sudo -u relaisdesk /opt/relaisdesk/api/relaisdesk-backup verify \
  -file /var/backups/relaisdesk/relaisdesk-AAAAMMJJTHHMMSSZ.db.aesgcm \
  -key-file /etc/relaisdesk/backup.key

# Vérifier une archive de factures (détection automatique du type).
sudo -u relaisdesk /opt/relaisdesk/api/relaisdesk-backup verify \
  -file /var/backups/relaisdesk/relaisdesk-AAAAMMJJTHHMMSSZ.invoices.aesgcm \
  -key-file /etc/relaisdesk/backup.key
```

Copier `/etc/relaisdesk/backup.key` une seule fois vers un coffre hors ligne,
avec des droits stricts. Ne jamais la placer dans le dépôt, dans la même
sauvegarde ou dans un courriel. Sans cette clé, les sauvegardes sont
irrécupérables. `BACKUP_MIRROR_DIR` permet d'écrire en plus vers un volume hors
machine monté sur `/mnt/relaisdesk-backups` ; le miroir doit être réellement
externalisé pour protéger d'une perte du serveur.

### Test de restauration

Le test ci-dessous écrit une nouvelle base sans toucher à la base active :

```bash
sudo -u relaisdesk /opt/relaisdesk/api/relaisdesk-backup restore \
  -file /var/backups/relaisdesk/relaisdesk-AAAAMMJJTHHMMSSZ.db.aesgcm \
  -out /data/relaisdesk/restore-test.db \
  -key-file /etc/relaisdesk/backup.key
```

Pour une restauration réelle, arrêter d'abord l'API, conserver l'ancienne base
et ses fichiers `-wal`/`-shm` dans un répertoire d'incident, puis installer la
base restaurée avec le propriétaire `relaisdesk` et le mode `0600`. Ne jamais
écraser `licences.db` pendant que l'API tourne.
