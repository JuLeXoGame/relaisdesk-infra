# Consignes du projet

## Modifications du site web — préférence explicite de l'exploitant

- Modifier directement les fichiers et assets du site dans `relaisdesk/`, en conservant leur arborescence. Ce dossier constitue la référence à transférer sur OVH.
- Ne plus créer systématiquement d'archives ZIP, de paquets de livraison ou de copies des fichiers web dans `output/`. N'en créer que sur demande explicite de l'utilisateur.
- À la fin d'une modification du site, indiquer brièvement les fichiers modifiés dans `relaisdesk/` et, si nécessaire, ceux à transférer sur OVH.
- Cette règle concerne le site web, pas les fichiers propres au serveur Oracle : ne pas placer l'API, les configurations serveur, les secrets ni les sauvegardes dans `relaisdesk/`.
- Ne pas supprimer les anciennes archives au seul motif de cette nouvelle préférence.
