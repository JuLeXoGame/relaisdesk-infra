# Mise en production des essais et du parcours B2C

## Résultat du 9 septembre 2026

La bascule Oracle s'est terminée à **16:13:15 UTC, soit 18:13:15 à Paris**. L'exploitant a expressément demandé Stripe LIVE et l'ouverture aux particuliers sans attendre le médiateur ni effectuer une recette Stripe TEST complète. Ce compte rendu décrit les contrôles techniques, **pas une certification juridique**.

- API Linux amd64 remplacée à `/opt/relaisdesk/api/api` ; service `relaisdesk-api` actif, sans redémarrage automatique observé après bascule.
- Essai **30 jours**, toutes les fonctionnalités et la capacité de l'offre choisie, carte vérifiée par Stripe ; paiement récurrent au tarif accepté après l'essai sauf annulation.
- B2C activé avec acquittement explicite du médiateur manquant. L'avertissement demeure visible : l'obligation n'est pas satisfaite par cet acquittement.
- Versions retournées par l'API : CGV `2026-09-09`, annexe essai `2026-09-09-trial-v2`.
- API Stripe LIVE et base existante conservées ; aucun faux paiement, abonnement ou client Stripe créé par le déploiement.
- Prix publiés vérifiés : Starter 24,90 EUR/mois ou 239 EUR/an ; Pro 110 EUR/mois ou 1 100 EUR/an ; personnalisé à partir de 199 EUR/mois ou 1 990 EUR/an.
- Les anciens documents contractuels archivés n'ont pas été réécrits.

## Sauvegarde et traçabilité

Dossier privé Oracle : `/opt/relaisdesk/deployments/trial-v2-20260909-9fmlcgvi`.

Sauvegarde : `/opt/relaisdesk/deployments/trial-v2-20260909-9fmlcgvi/rollback` (root:root, mode 0700), contenant notamment :

- `licences-before.db` : copie SQLite cohérente, service arrêté durant la copie, mode 0600 ; il ne s'agit pas d'une deuxième instance de service.
- ancienne API, ancien `api.env`, unité systemd et paramètres non secrets du webhook avant intervention.
- `migration-check.db` : copie utilisée pour valider la migration avant installation.

Le dossier privé contient aussi les nouveaux exécutables, les sources API/SQLite sans secrets, le script ponctuel et `deployment-result.json`.

| Artefact | SHA-256 |
| --- | --- |
| Ancienne API | `b67d90681c17e8d817b18a90a42a6ea96e0b009cca46fbbd472aa18e1f9b12c6` |
| API installée et processus exécuté | `fcb9cb1617266ee9c2b29376d77641119168391e932c5eaf4d33d96fa657b694` |
| Vérificateur SQLite livré | `83f3ddb962eaecb8973ba318467901373cef7e8958cecc816f9eebce5f7b2382` |
| Sources API/SQLite | `9b4d67b523ae201adec070b865907df2249dbfe5220527fb7c8a2aa8f10e8771` |
| Manifeste RustDesk, inchangé | `e2e0bb9d202d9c176f79e70a93b4f817e0b85013e420833878edf35c54a08bca` |

La configuration reste root:relaisdesk, mode 0640. Une clé HMAC anti-réessai a été générée si absente, sans l'afficher. La base est liée au mode `live` pour empêcher un changement accidentel vers TEST. Ne pas remplacer simplement les clés Stripe pour tester cette base.

## Webhook Stripe

Endpoint existant conservé : `we_1UCOgfLRTU0widUj3ofltjFE`, `https://api.relaisdesk.fr/api/v1/stripe/webhook`. Son secret de signature n'a pas été renouvelé et aucun second endpoint n'a été créé.

Événements activés et relus après mise à jour :

- `checkout.session.completed`
- `invoice.paid`
- `invoice.payment_failed`
- `customer.subscription.updated`
- `customer.subscription.deleted`

L'API a d'abord été démarrée avec les nouvelles souscriptions fermées, puis le webhook a été complété avant ouverture. Aucun prélèvement réel n'a été utilisé pour vérifier la livraison de ces événements.

## Contrôles réussis

- Tests Go sans cache des résultats et `go vet` : API, bibliothèque SQLite et intégration.
- Contrôles des contrats HTML/e-mail, archives historiques et 491 capacités personnalisées.
- Navigateur local isolé : choix d'offre, consentements et leur réinitialisation, ouverture B2C, demande d'essai, simulation de confirmation e-mail/Stripe, annulation, rétractation en deux étapes, affichage mobile. Tout trafic API/Stripe y est simulé.
- Régressions : pas de réactivation par une facture tardive après rétractation immédiate ; annulation et travail de reprise enregistrés dans une même transaction ; absence de fausse preuve d'envoi du rappel annuel lorsque SMTP n'est pas configuré ; arrêt du renouvellement si aucune preuve d'envoi n'est présente à J35.
- Migration et intégrité SQLite sur copie ; intégrité et clés étrangères sur la base après démarrage ; licences existantes inchangées et nombre de commandes conservé à ce stade.
- Depuis l'extérieur : `/api/v1/health` HTTP 200, `healthy`, base `connected` ; `/api/v1/public/trials` HTTP 200, `enabled=true`, `consumer_enabled=true`, `days=30` ; prix Pro 110/1 100 confirmés.
- CORS autorisé pour `https://relaisdesk.fr` et `https://www.relaisdesk.fr` ; écoute de l'API limitée à `127.0.0.1:8443`.
- Les 16 fichiers de `site-ovh-essai-v2.zip` ont finalement été vérifiés par empreinte HTTP, y compris `essai/`, `client/` et la dernière annexe RGPD.

## Ce qui n'a pas changé

Le site reste chez OVH. Nginx Oracle, l'unité systemd, les certificats et le DNS n'ont pas été modifiés par ce déploiement. Les conteneurs hbbs (`27be2b7e79bb`) et hbbr (`3c49df65fac7`) sont restés en place. Aucun binaire Windows ni manifeste de téléchargement n'a été publié ou remplacé.

## Actions restantes et limites

1. **OVH :** une vérification supplémentaire révèle deux anciens A vers `213.186.33.5` et une redirection `www` ajoutant `/www/`, aboutissant à une 404. Le correctif local `.htaccess` et `correctif-www-ovh.zip` sont prêts, mais pas publiés ; les suppressions DNS ne sont pas effectuées. Suivre `output/essai-production-2026-09-09/CORRECTIONS_OVH.md`. Le fichier a été contrôlé statiquement, sans serveur Apache local ; vérification OVH après publication indispensable.
2. **Médiateur :** sa désignation et la publication de ses coordonnées restent requises. L'ouverture choisie sans médiateur demeure non conforme sur ce point. [Obligation officielle](https://www.economie.gouv.fr/mediation-conso/vous-etes-un-professionnel/vos-principales-obligations-0).
3. **Recette réelle :** tests simulés et contrôles de santé ne remplacent pas une recette Stripe sandbox ni un essai de délivrabilité SMTP. Aucune carte fictive ne doit être utilisée sur le LIVE. L'absence de référencement ne rend pas la production privée.
4. **Exploitation :** les remboursements/avoirs et certains cas de rétractation nécessitent un traitement humain. Surveiller les notifications et travaux en échec avec les outils existants ; un envoi SMTP n'est pas une preuve de lecture et une panne complète du serveur peut empêcher les rappels et annulations de sécurité.

Procédures complémentaires : `docs/EXPLOITATION_ESSAIS_B2C.md` et `docs/REGISTRE_ESSAIS_ET_ABONNEMENTS.md`.

Ne pas relancer le script de déploiement après réussite. Ne pas restaurer aveuglément la base ou l'ancienne API dès que des abonnements existent : des paiements ou annulations postérieurs pourraient être perdus. En cas d'incident, fermer les nouvelles souscriptions tout en conservant la nouvelle API pour traiter les contrats déjà créés, puis préparer une intervention ciblée.
