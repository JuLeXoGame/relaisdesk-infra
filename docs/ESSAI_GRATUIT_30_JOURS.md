# Essai gratuit de 30 jours — préparation du 9 septembre 2026

> Ce document décrit la première préparation, conservée comme historique. La livraison suivante `2026-09-09-trial-v2` complète la rétractation et les rappels annuels, et prévoit l'ouverture B2C sur décision explicite de l'exploitant malgré l'absence de médiateur. Voir désormais [exploitation B2C](EXPLOITATION_ESSAIS_B2C.md), [registre RGPD](REGISTRE_ESSAIS_ET_ABONNEMENTS.md) et `output/essai-production-2026-09-09/README.md` pour l'état de déploiement. Les anciennes mentions « B2C fermé » ci-dessous décrivent la v1, pas la décision suivante.

## État réel

Implémentation locale, **non déployée et désactivée par défaut** (`TRIALS_ENABLED=false`). Les tarifs déjà en production ne sont pas modifiés. Aucun abonnement, débit, e-mail client, fichier Oracle, fichier OVH ou binaire RustDesk n'a été publié pendant ce travail.

L'essai est disponible sur toutes les capacités : Starter 1, Pro 5, Personnalisé 10 à 500 techniciens **simultanés**. Ce n'est pas une offre à 500 connexions pour tout le monde : l'utilisateur choisit la capacité qu'il essaie et le prix correspondant qu'il accepte après l'essai. Toutes les fonctionnalités de cette offre restent disponibles. Les installations ne sont pas limitées.

Après 30 jours : paiement et renouvellement automatiques mensuels ou annuels, sauf annulation. L'annuel est prélevé en une fois. La remise annuelle existante est distincte de l'essai et reste inchangée.

## Parcours mis en place

1. Choix de l'offre, de la capacité, de la périodicité et saisie des coordonnées. Le serveur recalcule le tarif et refuse une validation sur un prix devenu obsolète.
2. Acceptation distincte des CGV et du futur prélèvement, sans case précochée. Un changement d'offre exige une nouvelle acceptation.
3. Confirmation de l'e-mail par un lien à usage unique de 15 minutes. Le lien n'est pas consommé par un simple GET et son secret est retiré de la barre d'adresse.
4. Carte saisie **uniquement chez Stripe**, dans un Checkout en mode `setup` : aucun paiement ni abonnement à cette étape.
5. Après un webhook signé, l'API relit chez Stripe le Checkout, le SetupIntent réussi et le PaymentMethod. Elle contrôle leur rattachement au même client et à la même demande.
6. Réservation atomique du droit à l'essai par compte, e-mail et empreinte de carte pseudonymisée. Une demande refusée ne crée aucun abonnement facturable.
7. Création d'un abonnement Stripe avec une échéance d'essai fixe de 30 jours et émission d'une licence limitée à cette date. Les retries utilisent des clés d'idempotence et recherchent un abonnement déjà créé avant toute nouvelle création.
8. E-mail d'activation avec licence, tarif, date du premier prélèvement, lien d'annulation et copie des CGV + conditions particulières. Rappel prévu à J-7 par la file d'envoi existante.
9. Seule une facture Stripe réellement payée, au montant et pour la période attendus, prolonge l'accès. Un événement rejoué ne crée pas une deuxième commande et ne rallonge pas deux fois la licence. Une facture à zéro ne renouvelle pas le gratuit.
10. Annulation dans l'espace client, après récapitulatif, avec confirmation et e-mail. Les renouvellements manuels de cette licence sont bloqués tant que son abonnement récurrent n'est pas terminé, pour éviter une double facturation.

L'API reste légère : pas de nouveau broker, service de supervision ni dépendance Go. Les événements utilisent la file SQLite existante ; les rappels et la purge utilisent la maintenance horaire existante.

## Ce que l'anti-répétition garantit, et ses limites

- Annuler, ne pas renouveler, laisser expirer ou changer d'offre ne réinitialise pas les marqueurs.
- Un autre compte avec la même empreinte de carte est refusé. Deux demandes concurrentes sur cette carte ne peuvent pas toutes deux obtenir l'essai.
- Les numéros complets de carte, cryptogrammes, IBAN et mouvements bancaires ne sont ni reçus ni stockés par ce mécanisme. L'empreinte Stripe est transformée avec HMAC-SHA256 et une clé secrète distincte.
- La clé HMAC doit être sauvegardée. Une modification accidentelle est détectée et bloque les nouveaux essais, au lieu d'oublier silencieusement les cartes déjà utilisées.
- Une carte n'est pas une personne. Nouvelle carte, carte virtuelle, portefeuille tokenisé, autre e-mail ou autre compte peuvent contourner une partie du contrôle. Une carte d'entreprise partagée peut au contraire provoquer un faux positif : prévoir un examen humain via le formulaire de contact. [Limites de l'empreinte Stripe](https://docs.stripe.com/api/payment_methods/object?lang=dotnet).
- Le SIRET est conservé pour la facturation mais ne sert pas à bloquer automatiquement une entreprise : il est public et auto-déclaré, et ne prouve pas que son utilisateur représente cette entreprise.
- Les anciens clients sont reconnus par l'historique des licences du compte/e-mail. Les anciens paiements n'ont pas tous un historique local d'empreintes de carte : cette livraison n'a pas récupéré rétrospectivement leurs cartes dans Stripe.
- Les marqueurs sont conservés pendant l'abonnement puis trois ans après sa fin, et purgés automatiquement. Ce choix interne doit être documenté et réévalué dans le registre RGPD ; ce n'est pas une durée imposée par la CNIL. Une garantie anti-retour perpétuelle serait incompatible avec l'effacement de tout historique. [Principe de limitation de conservation](https://www.cnil.fr/fr/passer-laction/les-durees-de-conservation-des-donnees).

## Avant toute activation publique

### 1. Tester hors production

Utiliser une **base de test distincte**, des clés Stripe TEST, un webhook TEST et une boîte e-mail contrôlée par vous. Ne jamais mélanger les objets TEST et LIVE dans la même base de licences. Aucun test réel Stripe n'a été lancé pendant cette livraison : les tests de code utilisent un faux serveur Stripe et le navigateur intercepte toutes les requêtes d'API.

Rejouer les scénarios suivants dans le bac à sable Stripe, avec les moyens de paiement de test officiels :

- Starter, Pro, Personnalisé 10 et 500 ; mensuel et annuel ; tarif total et capacité exacts.
- E-mail non confirmé, lien expiré, lien rejoué, vérification bancaire interrompue/3-D Secure refusée.
- Activation : une licence, un abonnement, aucune somme débitée ; accès à l'espace client et création du mot de passe par « Mot de passe oublié ».
- Même compte avec autre carte, puis autre compte avec même carte : pas de deuxième essai ni d'abonnement supplémentaire.
- Annulation pendant l'essai : confirmation et e-mail avec date de fin ; aucun débit à J30 ; accès maintenu jusqu'à cette date.
- Premier débit réussi, échec puis régularisation, échéance mensuelle/annuelle ; facture locale, e-mail et période d'accès corrects.
- Webhooks répétés, reçus dans le désordre, reprise après arrêt/redémarrage de l'API ; pas de duplication.
- Stripe indisponible lors de l'annulation : demande sauvegardée, réponse explicitement « en attente », reprise automatique. Vérifier manuellement dans Stripe si l'échéance est proche ; traiter les éventuels paiements déjà engagés et remboursements selon les droits du client.
- Sur mobile : montant annuel en une fois visible, cases non cochées, bouton d'annulation accessible.

Les horloges de test peuvent accélérer les échéances d'abonnements de sandbox compatibles ; elles ne remplacent pas une recette complète du Checkout. Ne pas réduire les 30 jours dans les sources de production pour tester. [Tests Stripe Billing](https://docs.stripe.com/billing/testing).

### 2. Configurer Stripe

Vérifier l'activation de Stripe Billing et ses éventuels frais. Le premier abonnement crée un produit technique `relaisdesk_subscription_v1` puis son prix récurrent calculé côté serveur. Ne pas modifier les prix, quantités ou durées de ces abonnements depuis Stripe sans adapter leur contrat local : un écart de montant/période est refusé pour l'octroi d'accès.

Conserver l'endpoint existant, confirmé dans `api/main.go` :

`https://api.relaisdesk.fr/api/v1/stripe/webhook`

Sélectionner les événements :

- `checkout.session.completed`
- `invoice.paid`
- `invoice.payment_failed`
- `customer.subscription.updated`
- `customer.subscription.deleted`

Les objets relus par l'API utilisent `Stripe-Version: 2024-06-20`. Ne pas modifier aveuglément la version globale du compte. Le webhook peut avoir une version différente : seuls les identifiants nécessaires sont extraits, puis l'objet est relu avec la version prévue.

Activer/configurer les e-mails Stripe de rappel de fin d'essai, de reçus et de régularisation de paiement. Le rappel local J-7 n'est pas une garantie de réception et ne remplace pas la vérification des exigences des réseaux de cartes. [Rappels Stripe](https://docs.stripe.com/billing/revenue-recovery/customer-emails), [événements d'abonnement](https://docs.stripe.com/billing/subscriptions/webhooks).

Configurer aussi l'action finale après les tentatives de recouvrement : annuler les abonnements définitivement impayés, plutôt que les laisser indéfiniment en `past_due`. RelaisDesk n'accorde aucune prolongation gratuite pendant le recouvrement ; un délai de traitement Stripe peut donc créer une courte interruption à l'échéance jusqu'au paiement confirmé.

### 3. Préparer les secrets et la sauvegarde

Dans `/etc/relaisdesk/api.env` sur Oracle, **sans remplacer les autres paramètres** :

```dotenv
TRIALS_ENABLED=false
TRIAL_FINGERPRINT_KEY=<32 octets aléatoires encodés en base64url, sans signe égal>
```

Générer cette clé localement avec un générateur cryptographique, la conserver dans votre gestionnaire de secrets et dans la sauvegarde chiffrée. Ne pas réutiliser l'ADMIN_TOKEN, la clé SSH, la clé réseau ou la clé de signature du manifeste. Ne pas la publier sur GitHub ni dans le dossier de téléchargement.

Avant de remplacer l'API, sauvegarder le binaire, la configuration et SQLite de façon cohérente ; tester la migration sur une copie avec `relaisdesk-dbcheck`. Le nouveau schéma ajoute des tables sans changer les anciennes commandes ni leur prix.

### 4. Publier les pages OVH, puis activer l'API

Le site reste sur OVH. Oracle ne reçoit que l'API et ses fichiers de configuration habituels. Fichiers web à envoyer :

- `relaisdesk/essai/` (page, script, styles et conditions particulières) ;
- `relaisdesk/trial-offer.js`, `relaisdesk/index.html`, `relaisdesk/app.js` et `relaisdesk/i18n.js` ;
- `relaisdesk/client/index.html` et `relaisdesk/client/app.js` ;
- `relaisdesk/cgv.html` et `relaisdesk/politique-confidentialite.html`.

La feuille existante `relaisdesk/client/styles.css` doit être présente sur OVH. La bannière d'essai reste masquée si l'API est absente ou désactivée.

Après validation de la recette, déployer l'API, vérifier le HTTPS et les journaux, puis passer `TRIALS_ENABLED=true` et redémarrer l'API. Vérifier `GET /api/v1/public/trials`, le lien vitrine et l'espace client. Ne pas réexécuter le script de déploiement des prix du 9 septembre : il est dédié à une autre version.

**Désactivation :** remettre `TRIALS_ENABLED=false` ferme les nouvelles souscriptions, mais **n'annule pas** les abonnements Stripe existants. Conserver leurs webhooks, la clé HMAC et le traitement des annulations/factures. Ne pas restaurer une ancienne API dépourvue de ces gestionnaires après avoir créé des abonnements récurrents.

### 5. Juridique et exploitation

Les CGV historiques du 8 septembre sont conservées. L'essai a un complément distinct `2026-09-09-trial`, transmis avec les CGV par e-mail. Ajouter le traitement anti-abus au registre RGPD : finalité, intérêt légitime, tests de proportionnalité, accès limités, durée de trois ans à réévaluer et procédure de contestation humaine.

Les essais B2C sont techniquement fermés dans cette livraison, y compris si le réglage des ventes prépayées B2C est activé ultérieurement. Toutes les capacités sont disponibles pour les professionnels. Ne pas ouvrir les essais B2C au seul motif que Stripe fonctionne : médiateur, rétractation en ligne (y compris pendant l'essai sans commande payée), reconduction annuelle et rappels légaux doivent être implémentés et validés avant cette ouverture spécifique. Les changements ne constituent pas une validation juridique complète. [Information contractuelle et résiliation en ligne — DGCCRF](https://www.economie.gouv.fr/dgccrf/les-fiches-pratiques/e-commerce-les-regles-entre-professionnels-et-consommateurs).

En cas de jobs en échec définitif, reprendre l'événement après diagnostic. Une création d'abonnement retardée de plus de 30 minutes est suspendue si aucun abonnement correspondant n'existe déjà : ne jamais recréer manuellement un autre essai à l'aveugle. Les paiements partiels, remises manuelles et proratisations sont volontairement refusés par cette première version, qui attend le montant contractuel complet ; les traiter explicitement avant de modifier un contrat existant.

Une révocation technique de licence ne résilie pas automatiquement son contrat Stripe. Si le service est définitivement interrompu, traiter également la résiliation commerciale et les éventuels remboursements ; ne pas laisser un abonnement continuer à prélever pour un service que vous avez supprimé.

## Vérifications locales

```powershell
cd api
go test -count=1 -trimpath -mod=readonly ./...
cd ../database
go test -count=1 -trimpath -mod=readonly ./...
cd ../tests
go test -count=1 -trimpath -mod=readonly ./...
cd ..
node scripts/pricing-checks.mjs
node scripts/legal-checks.mjs
node scripts/trial-contract-checks.mjs
# Avec Playwright installé et Microsoft Edge :
node scripts/trial-ui-checks.cjs
```

Les contrôles navigateur sont isolés de la production. Les captures et fichiers de cette préparation historique sont conservés dans `archives/livraisons/essai-30-jours-2026-09-09/`. Les nouvelles exécutions de `scripts/trial-ui-checks.cjs` écrivent dans `output/tests/parcours-essai/`, sans modifier les anciennes livraisons. Aucun exécutable client/technicien RustDesk n'est concerné.
