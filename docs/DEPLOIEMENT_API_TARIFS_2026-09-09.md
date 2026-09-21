# API Oracle — déploiement des tarifs du 9 septembre 2026

## Résultat

L'API a été mise à jour le **9 septembre 2026 à 06:02:14 UTC / 08:02:14 Europe/Paris**, à la demande explicite de l'exploitant.

Le catalogue public `https://api.relaisdesk.fr/api/v1/public/pricing` a confirmé :

| Offre | 30 jours | 365 jours | Capacité |
| --- | ---: | ---: | --- |
| Starter | 24,90 € | 239 € | 1 technicien simultané |
| Pro | 110 € | 1 100 € | 5 techniciens simultanés |
| Personnalisé — base | 199 € | 1 990 € | 10 techniciens simultanés, extensible à 500 |

La nouvelle API embarque les CGV `2026-09-08` ainsi que les archives `2026-08-25` et `2026-09-07`. Les pages publiques OVH `app.js`, `client/app.js` et `cgv.html` utilisaient déjà la version `2026-09-08` avant l'activation. Les constantes tarifaires du JavaScript public correspondaient aux nouveaux montants.

## Périmètre strict

- Seul le binaire `/opt/relaisdesk/api/api` a été remplacé, avec arrêt/démarrage du service `relaisdesk-api`.
- Le fichier `/etc/relaisdesk/api.env` et l'unité systemd sont inchangés, empreintes contrôlées avant/après.
- Nginx, DNS, certificats, secrets et règles de pare-feu n'ont pas été modifiés.
- Les conteneurs hbbs/hbbr conservent leurs identifiants et l'image `relaisdesk/rustdesk-server:d998a32-security-amd64`.
- Le manifeste des téléchargements est inchangé. La version publique reste **`1.0.0-legal.20260907`** ; les paquets clients `1.0.1` en attente de recette ne sont pas publiés.
- Aucune écriture sur OVH, aucun push GitHub et aucun envoi ANSSI pendant ce déploiement.
- Aucun achat réel ou fictif n'a été créé sur le serveur pour la recette. Les tests de paiement ont utilisé des bases temporaires et des services simulés en local.

## Construction et provenance

Les sources locales de `api/` et `database/` ont été compilées pour `linux/amd64`, Go **1.26.6**, `CGO_ENABLED=0`, `-trimpath`, `-mod=readonly`, `-ldflags '-s -w'`.

Les fichiers de cette livraison historique se trouvent désormais dans `archives/livraisons/api-pricing-2026-09-09/` (rangement du 9 septembre 2026) :

| Fichier | SHA-256 |
| --- | --- |
| API déployée `api` | `b67d90681c17e8d817b18a90a42a6ea96e0b009cca46fbbd472aa18e1f9b12c6` |
| Ancienne API sauvegardée | `e04410a896c40047aa35c8156e2c531dd157263d9cf3969a2b49ece82f945ba2` |
| Contrôle SQLite `relaisdesk-dbcheck` | `6770bbfbf87722c861cd27321d392304820d8e0e33ae6885ef30c5197b321a22` |
| Archive des sources `sources-api.tar.gz` | `6beb5391e33835c6966ec66da557b3a9f78bfbeb0c1f7fb9730904ee52133491` |
| Script ponctuel `activate-api.sh` | `81d6a889b917883f52bb57d535293ca0fdc2465c86dcc0332028ae520aca032a` |

L'archive comprend 90 fichiers sources Go, modules, sommes, schémas SQL et textes contractuels. Elle ne contient pas de `.env`, de clé privée ni de base client. La copie `api-with-symbols.audit` et l'auditeur placé dans `tools/` sont des outils de contrôle locaux, pas des fichiers à déployer.

## Contrôles effectués

- Tests API, base et intégration relancés avec `-count=1 -trimpath -mod=readonly` : réussis.
- `scripts/pricing-checks.mjs` : toutes les capacités Personnalisé de 10 à 500, affichage, annuel et comparaisons Starter/Pro vérifiés.
- `scripts/legal-checks.mjs` : cohérence des versions contractuelles, prix et archives vérifiée.
- Empreintes des binaires et de l'archive comparées avant/après transfert SSH.
- Sauvegarde SQLite cohérente réalisée avant la bascule ; intégrité vérifiée.
- Exécution du nouveau `relaisdesk-dbcheck` sur une **copie** de cette sauvegarde : migration/intégrité OK. Le schéma obtenu est identique au schéma sauvegardé.
- Après activation : `PRAGMA integrity_check` et `PRAGMA foreign_key_check` OK. Les montants et versions contractuelles des commandes historiques encore présentes sont inchangés ; les commandes payées de la sauvegarde sont conservées.
- Santé publique : `status=healthy`, `database=connected`.
- Catalogue public vérifié depuis Windows, en dehors du VPS, avec les quatre nouveaux montants et Starter inchangé.
- PID observé `244977`, état `active/running`, `NRestarts=0`, écoute uniquement sur `127.0.0.1:8443`. L'empreinte du binaire réellement exécuté dans `/proc/<PID>/exe` correspond au nouveau binaire.

Le champ `version=1.0.0` de `/health` est une constante applicative existante, pas un identifiant de cette compilation. La preuve de livraison est l'empreinte du processus actif et le catalogue renvoyé, pas ce champ.

### Qualification de l'avis OpenPGP

L'audit du binaire allégé remonte GO-2026-5932 avec des symboles génériques OpenPGP. Les auditeurs Go 1.7.0 et 1.8.0 peuvent revenir à la précision du module lorsque les symboles ne sont pas extractibles ; cela ne démontre pas l'import de ces paquets. Cette limite est documentée dans [govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck#hdr-Limitations).

Vérifications complémentaires avec la configuration de compilation Linux :

- `go list -deps ./...` : **245 paquets, aucun `golang.org/x/crypto/openpgp`**.
- Analyse des sources : zéro vulnérabilité dans les symboles appelés et les paquets importés ; un avis au niveau du module `x/crypto`.
- Construction de contrôle conservant les symboles (`-ldflags '-w'`), puis audit binaire avec govulncheck 1.8.0 : zéro vulnérabilité dans les symboles analysés ; seul l'avis module reste mentionné.

L'avis n'a pas été supprimé du rapport et aucune exclusion globale n'a été ajoutée. Ces résultats ne sont pas une garantie d'absence de toute faille.

## Sauvegarde et retour arrière

Dossier privé, appartenant à root, mode `0700` :

```text
/opt/relaisdesk/deployments/api-pricing-20260909-4ctcixKC/rollback
```

Il contient l'ancienne API, l'environnement, l'unité systemd, la sauvegarde `licences-before-api.db` en mode `0600` et la copie de vérification de migration. **Ne pas télécharger ni publier ce dossier**, notamment son environnement et ses bases.

Le script d'activation ponctuel prépare un retour arrière automatique du binaire si ses contrôles après bascule échouent. Ce retour arrière n'a pas été nécessaire. Il ne restaure jamais automatiquement une ancienne base, pour ne pas effacer les écritures récentes.

Pour un retour arrière ultérieur, vérifier d'abord les empreintes et la version réellement active, puis réinstaller uniquement l'ancien binaire sauvegardé et redémarrer l'API. Coordonner aussi les prix et CGV du site : revenir à l'ancienne API seule réintroduirait une incohérence avec les pages actuelles. Le script `activate-api.sh` est une opération ponctuelle, pas un outil à relancer après ce déploiement.

## Incident préalable sans modification du service

La première tentative s'est arrêtée lors de la vérification HTTPS du site OVH (`Connection reset by peer`), avant la création du dossier de sauvegarde et avant toute modification du service. L'ancienne API est restée active avec son empreinte d'origine. Après réussite d'un nouveau contrôle HTTPS, la seconde tentative a terminé avec tous les contrôles positifs. Aucune modification DNS ou TLS n'a été faite pour contourner cet incident.
