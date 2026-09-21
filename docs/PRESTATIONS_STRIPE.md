# Connexion directe et Connexion & Prestation

## Parcours demandé

Implémentation locale du 19 septembre 2026, **désactivée par défaut et non déployée**. Les sources de l'API, du portail client, du lanceur technicien et du moteur contiennent le nouveau parcours. Les programmes actuellement distribués ne sont pas remplacés. La recette avec de vrais postes et Stripe en mode test reste obligatoire avant activation commerciale.

- **Connexion directe** : conserve la prise en main actuelle, sans création de paiement.
- **Connexion & Prestation** : le technicien sélectionne dans l'application une prestation du catalogue de son entreprise avant la prise en main.
- L'administrateur de l'entreprise cliente, et non l'administrateur général RelaisDesk, active ou désactive l'option et gère ce catalogue dans son espace client.
- Le catalogue propose des forfaits prépayés et plusieurs tarifs horaires. Un technicien membre sélectionne un tarif autorisé, sans pouvoir modifier son prix ni le compte bénéficiaire.
- L'option reste désactivée par défaut. Sa désactivation empêche de nouvelles prestations, sans faire disparaître les prestations et paiements existants.

## Forfait prépayé

1. Sélection de la prestation et présentation du montant au client.
2. Génération côté serveur d'un paiement Stripe Checkout sur le compte Stripe connecté de l'entreprise prestataire.
3. Transmission d'un lien au client ; saisie bancaire exclusivement dans le navigateur externe, sur la page hébergée par Stripe, ou sur son téléphone.
4. Confirmation serveur du paiement : signature du webhook, environnement test/live, compte connecté, session, montant, devise et état payé vérifiés.
5. Le parcours « Connexion & Prestation » permet ensuite de lancer la prise en main. Ni le retour navigateur ni une déclaration du technicien ne valent preuve de paiement.

La connexion directe demeure un parcours indépendant : il ne s'agit pas d'un verrou universel du moteur RustDesk.

## Tarif horaire

1. Sélection du tarif avant la prise en main ; information et accord du client sur le tarif et le mode de calcul.
2. Réalisation de la prestation : seuls les intervalles de prise en main réellement établis sont comptabilisés.
3. Arrêt de la facturation, clôture de la prise en main, présentation de la durée et du montant final.
4. Création du paiement Checkout et règlement par le client après la prestation. Aucun débit automatique de carte mémorisée.

**Décision de l'exploitant** : facturer uniquement les minutes connectées au poste. Aucun chronomètre de travail manuel ni temps de travail hors connexion.

### Règles de comptage

- Début après authentification et acceptation de la session de contrôle distant, jamais au clic du bouton, au lancement de RustDesk ou à la seule ouverture d'une liaison réseau.
- Fin à la fermeture de la session. Une coupure suspend le comptage ; une reconnexion réussie ouvre un nouvel intervalle sur la même prestation tant qu'elle n'est pas clôturée.
- Sont exclus : attente de mot de passe ou d'accord du client, tentatives échouées, attente de réveil du poste, redémarrages, intervalles déconnectés, attente du paiement et présence d'un agent permanent sans prise en main.
- La facturation dépend d'une connexion établie, pas de l'activité souris/clavier : regarder le poste sans déplacer la souris reste du temps connecté.
- Une prestation concerne un seul poste. L'implémentation refuse une deuxième connexion simultanée sur la même prestation ; elle ne somme donc pas deux fenêtres. Les canaux auxiliaires (audio, presse-papiers, transfert de fichiers), ainsi que les sessions autonomes terminal/caméra/tunnel, ne déclenchent pas de compteur de contrôle distant.
- Calcul implémenté : conserver une précision inférieure à la minute, cumuler les intervalles, appliquer le tarif horaire au prorata, puis arrondir une seule fois le montant final au centime. Pas de minute commencée facturée automatiquement, ni d'arrondi répété à chaque reconnexion. Cette règle doit être présentée au client avec le tarif avant la connexion.
- Une fermeture du logiciel, une mise en veille, un plantage ou une perte réseau ne doit jamais laisser un compteur courir indéfiniment. Employer une horloge monotone pour les durées locales et des confirmations périodiques de présence du pair. En l'absence de fin explicite, s'arrêter à la dernière borne confirmée, sans facturer la période d'incertitude ou le délai d'expiration technique.
- Les événements doivent être liés à la prestation, au poste, au technicien et à un identifiant de connexion. Dédupliquer les événements, rejeter les intervalles incohérents et empêcher de rouvrir une prestation clôturée ou déjà mise en paiement.
- Avant d'émettre le paiement horaire, fermer les connexions de la prestation et figer la durée et le montant côté serveur. Un événement retardé ne doit pas modifier le montant présenté dans Checkout. Une durée nulle ne génère pas de paiement ; un éventuel minimum imposé par Stripe ne doit pas devenir une majoration silencieuse.

### Points d'intégration identifiés dans le code

Le lanceur appelle `launchRustDeskSession` dans `installer/configurator/rustdesk_windows.go` et ses équivalents. Le nouveau module `service_billing.go` ouvre une passerelle locale sur `127.0.0.1`, avec un secret aléatoire distinct de la session API. Son fichier de liaison par poste est protégé par les permissions du compte Windows ou Unix. La durée d'exécution du processus RustDesk n'est jamais utilisée comme durée facturable.

Dans `rustdesk/src/client/io_loop.rs`, le compteur est raccordé à la réception de `LoginResponse::PeerInfo`, puis aux messages valides du pair authentifié. Son état est réinitialisé à chaque tentative et libéré à la fermeture. Les modules `relaisdesk_meter.rs` et `relaisdesk_meter_clock.rs` transmettent des durées monotones, un numéro de séquence et un identifiant de connexion à la passerelle, qui les soumet à l'API avec la session du technicien. Le moteur n'obtient pas ce jeton API.

Le choix de mesure est conservateur : un intervalle entre confirmations du pair supérieur à 3 secondes est exclu ; l'API exclut aussi un intervalle de mesure reçu après plus de 6 secondes ou avec une séquence manquante. Un échec de transmission du compteur fait fermer la connexion concernée. Une veille, une perte Internet ou l'arrêt du lanceur peuvent donc retirer quelques secondes non confirmées du total, jamais ajouter tout le délai d'expiration. Il faut mesurer ce comportement sur de vrais postes avant commercialisation.

Les durées sont produites par le moteur et transmises par une session authentifiée. Ce n'est **pas une attestation indépendante inviolable** contre un technicien qui modifie volontairement son logiciel. Les contrôles serveur bornent les durées, les séquences, les doublons et les droits, mais ne prouvent pas à eux seuls l'honnêteté d'un moteur modifié.

Ne pas déduire la durée de `hbbs`, du renouvellement de licence réseau ou de la présence d'un agent en ligne : ce ne sont pas des preuves de prise en main. Le comptage doit fonctionner aussi bien en connexion directe qu'en connexion relayée par `hbbr`.

### Recette obligatoire avant activation

- Refus, mauvais mot de passe, attente d'acceptation et échec réseau : durée nulle.
- Connexion puis fermeture normale : une seule période facturée.
- Deux périodes séparées par une coupure : cumul des périodes seulement.
- Plusieurs reconnexions courtes : pas de minutes ajoutées par arrondis successifs.
- Double notification, notification tardive et deux fenêtres simultanées : aucune double facturation.
- Plantage, perte Internet, veille et redémarrage : aucune période non confirmée ajoutée après le dernier point fiable.
- Changement d'heure système : pas d'allongement ou de durée négative.
- Connexion directe sans prestation : aucun paiement ni temps commercial enregistré.
- Fermeture et paiement : toutes les prises en main de la prestation sont closes avant la saisie bancaire ; aucun débit automatique pour l'horaire.

## Sécurité et séparation des comptes

- Paiements directs Stripe Connect sur le compte de l'entreprise prestataire, distincts des abonnements RelaisDesk. Aucun compte bénéficiaire arbitraire fourni par le logiciel client.
- Inscription et coordonnées de versement de l'entreprise recueillies par Stripe ; aucun secret Stripe personnel saisi dans l'application.
- Montants en centimes calculés côté serveur. Tarif figé sur la prestation, indépendant des modifications futures du catalogue.
- Autorisations contrôlées sur chaque opération : entreprise, licence, membre et dossiers accessibles ; ne pas élargir les droits existants sur l'historique commercial.
- Création de paiement idempotente, traitement des notifications en double, expiration et reprise après panne sans double facturation.
- Aucun numéro complet de carte ni cryptogramme dans les exécutables, l'API, les journaux ou la base RelaisDesk.
- Pas de partage d'écran, de contrôle distant ni d'enregistrement pendant la saisie bancaire. Le simple lancement d'un navigateur externe ne suffit pas à garantir cette isolation.
- Pas d'affirmation de conformité PCI DSS automatique : confirmer avec Stripe les obligations de la plateforme et des entreprises prestataires avant activation commerciale.

## État local

Les migrations ajoutent seulement les tables `service_merchants`, `service_rates` et `service_work` et leurs index. Le journal de prestations est distinct de l'historique d'interventions manuel : ce dernier ne fournit aucune durée facturable.

Contrôles automatisés disponibles : calcul et arrondis, silence et reprise, doublons, prépaiement obligatoire, prix figés, séparation entre entreprises, restrictions des membres par dossier, révocation des postes, authentification de la passerelle locale, rejet des liens bancaires étrangers, signature et contrôle des notifications Stripe, idempotence de création du paiement et limites de trafic. Les appels Stripe de ces tests sont simulés. Une vérification de compilation ne remplace pas la recette complète ci-dessus.

### Configuration après validation des candidats

1. Activer **Stripe Connect** sur le compte plateforme et vérifier auprès de Stripe l'adéquation des paiements directs avec des comptes Standard à votre activité. Ne pas remplacer le webhook des abonnements RelaisDesk.
2. Commencer avec la clé **test** et un webhook **pour les événements des comptes connectés** :
   `https://api.relaisdesk.fr/api/v1/stripe/connect-webhook`.
   Événements traités : `checkout.session.completed` et `checkout.session.async_payment_succeeded`.
3. Configurer dans l'environnement de l'API, et uniquement côté serveur :

   ```dotenv
   SERVICE_PAYMENTS_ENABLED=false
   STRIPE_CONNECT_WEBHOOK_SECRET=whsec_...
   ```

   Le module utilise `STRIPE_SECRET_KEY` et `PUBLIC_WEBSITE_URL` existants. Il refuse `STRIPE_MOCK=true`. **Ne pas modifier les clés de la production pour un essai local** : employer une instance de test et une base dédiée pour valider ce nouveau flux.
4. Préparer un moteur RustDesk et un lanceur compatibles. `rustdesk --version` doit contenir `billing-meter-v1`. Le lanceur refuse de commencer une prestation avec un ancien moteur. Fermer toutes les anciennes instances avant les essais, car RustDesk peut réutiliser un processus déjà lancé. Ne pas diffuser un lanceur recompilé avec l'ancien moteur embarqué.
5. Prévoir ensemble les fichiers web `relaisdesk/client/index.html`, `app.js`, `teams.js`, `services.js` et `relaisdesk/paiement-prestation.html`. Aucun ZIP n'est nécessaire. Le dossier `relaisdesk/` reste la source à transférer sur OVH.
6. Sur l'instance de recette uniquement, activer `SERVICE_PAYMENTS_ENABLED=true`, redémarrer l'API, puis ouvrir **Prestations & paiements clients** dans le compte du propriétaire de licence. Configurer son compte Stripe, terminer l'onboarding, actualiser et activer les prestations. Ajouter les tarifs en **prix total payable**, par heure pour l'horaire. Un membre d'équipe ne peut pas gérer ce catalogue ni le compte bénéficiaire.
7. Effectuer toute la recette, vérifier le compte Stripe réellement bénéficiaire et les montants, puis organiser séparément la publication des candidats et l'activation en production. Rien n'est activé ou publié automatiquement par ces changements.

Le bouton du propriétaire désactive les **nouvelles** prestations de son entreprise, tout en conservant les prestations déjà ouvertes et leur règlement. Le commutateur serveur est un arrêt global : les compteurs ne sont alors plus acceptés ; le propriétaire peut encore consulter et clôturer les dossiers existants. Les webhooks restent traités pour ne pas perdre un paiement en cours.

### Limites explicites de cette première version

- Encaissements uniques en EUR, sans commission RelaisDesk ajoutée par ce code, ni prélèvement automatique de la carte pour la prestation horaire. Les conditions et frais de Stripe restent applicables.
- Le lien est copié par le technicien, puis communiqué au client par le moyen de son choix. Il n'y a pas d'envoi automatique par email ou SMS dans ce module.
- 100 tarifs actifs maximum, chaque tarif entre 0,50 € et 10 000 €. Les prix sont figés dès la préparation de la prestation ; changer le catalogue ne change pas une prestation existante.
- Maximum technique de 7 jours de connexion cumulée pour une prestation. L'interface présente les 100 dernières prestations et jusqu'à 200 tarifs, en donnant priorité aux tarifs actifs.
- Un total horaire inférieur à 0,50 € ne crée pas de Checkout ; il n'est jamais relevé artificiellement. Le propriétaire doit le gérer hors de ce parcours ou ne pas l'encaisser.
- Un lien Checkout expiré ou une création dont le résultat est incertain depuis plus de 23 heures bloque toute nouvelle création automatique. Réconcilier dans Stripe avant une action manuelle, pour éviter un double paiement. Le module ne régénère pas encore ces liens.
- Factures de prestation, remboursements, litiges et suivi des remboursements sont gérés dans les outils de l'entreprise et dans Stripe. Le statut local « payé » n'est pas un registre comptable complet des remboursements.
- La case d'accord est une déclaration du technicien, pas une signature électronique du client ni une gestion complète de ses conditions contractuelles et droits éventuels de rétractation. Préparer ces éléments avec l'entreprise prestataire avant utilisation commerciale.
- Le logiciel ferme les connexions suivies par sa passerelle, pas tous les autres outils de prise en main éventuellement ouverts sur le poste. Le client doit payer sans partage d'écran actif, ou sur un appareil personnel distinct. Ouvrir Checkout dans un navigateur ne garantit pas, à lui seul, l'absence de visibilité du technicien.
- La mesure émet des confirmations toutes les 2 secondes uniquement pendant les parcours de prestation. La protection de trafic est proportionnée à la capacité de la licence ; une recette de charge reste nécessaire avant un grand nombre de prestations simultanées sur le petit VPS.
- Les mentions contractuelles et de confidentialité du service devront couvrir ces nouvelles métadonnées de prestations, leur conservation et Stripe Connect avant activation publique. Aucun label PCI DSS ni validation juridique n'est déduit des tests techniques.

### Documentation Stripe de référence

- [Paiements directs avec Checkout hébergé](https://docs.stripe.com/connect/direct-charges?platform=web&ui=stripe-hosted)
- [Comptes Standard](https://docs.stripe.com/connect/standard-accounts)
- [Liens d'onboarding](https://docs.stripe.com/api/account_links/create)
- [Webhooks Connect](https://docs.stripe.com/connect/webhooks)
