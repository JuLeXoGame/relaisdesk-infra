# RelaisDesk — prérequis juridiques avant mise en production

Dernière mise à jour : 21 septembre 2026 (CGV `2026-09-21`, essai `2026-09-21-fleet-v2` : paiement en crypto-actifs). Le [suivi actuel](SUIVI_JURIDIQUE_2026-09-12.md)
distingue les corrections web des pièces externes restant à obtenir ; cette livraison-ci modifie l'API et le web de façon coordonnée (nouvelle version acceptée, déploiement via `deploy_api_legal_update.sh`). Pour la console de parc, consulter
[JURIDIQUE_PARC_ET_ACCES_PERMANENT.md](JURIDIQUE_PARC_ET_ACCES_PERMANENT.md) : procédures manuelles à appliquer et historique de préparation. Les conditions du 11 septembre sont désormais publiées ; celles du 21 septembre sont préparées. Voir aussi les décisions historiques et le dossier
ANSSI rectificatif dans [RECTIFICATIFS_JURIDIQUES_2026-09-07.md](RECTIFICATIFS_JURIDIQUES_2026-09-07.md).

Ce document est une checklist opérationnelle, pas une consultation juridique.
Les ventes ne doivent être ouvertes qu'après traitement des points bloquants.
Pour savoir **quand** effectuer chaque action par rapport à la migration Oracle,
utiliser le guide chronologique
[`GUIDE_JURIDIQUE_MIGRATION_PRODUCTION.md`](GUIDE_JURIDIQUE_MIGRATION_PRODUCTION.md).

## Points déjà intégrés au projet

- Identité du vendeur : Julien BELLOT, entrepreneur individuel, SIREN
  940 747 108, SIRET 940 747 108 00014, code APE 9511Z, siège à Bizeneuille.
- Téléphone professionnel publié : 06 62 85 59 30.
- Mention « TVA non applicable, art. 293 B du CGI » sur les prix, les CGV et
  les factures.
- CGV distinctes pour professionnels et consommateurs, commande avec
  obligation de paiement, conservation de la version acceptée et demande
  expresse de commencement immédiat pour un consommateur.
- Copie datée des CGV jointe aux e-mails de commande et d'activation.
- Fonction de rétractation en ligne avec référence et horodatage, plus accusé
  de réception par e-mail.
- Politique de confidentialité et durées de conservation. Certaines données
  opérationnelles sont purgées automatiquement ; les fiches de parc nécessitent
  encore la procédure manuelle décrite dans le complément du 11 septembre.
- Accès permanent : autorisation préalable documentée, information des utilisateurs,
  retrait des accès, données de parc et limites des suppressions précisés dans les CGV/DPA.
- Avis de fork, attribution AGPLv3 et page dédiée au code source.
- Ventes B2C ouvertes à la demande de l'exploitant ; médiateur non encore désigné.
  Ce point reste non régularisé et ne peut pas être corrigé par une simple mention.
- Téléchargements non conditionnés à SignPath. Sources AGPL correspondantes,
  licences, manifeste Ed25519 et information honnête sur Authenticode restent requis.

## Blocages à lever avant la mise en production complète

1. **Médiateur de la consommation.** Adhérer à un médiateur référencé par la
   CECMC et compétent pour l'activité. Publier son nom, son adresse postale et
   son site dans `relaisdesk/cgv.html` et dans la copie des CGV jointe aux
   e-mails, puis définir
   `CONSUMER_MEDIATOR_NAME` et `CONSUMER_MEDIATOR_URL`. Le médiateur ne peut pas
   être inventé ni choisi sans convention préalable.

2. **Forks publics et sources correspondantes.** Les dépôts client, serveur et
   protocole sont publiés sur le compte `JuLeXoGame`, avec les URL immuables
   renseignées dans `relaisdesk/logiciel-libre.html`, `CLIENT_SOURCE_URL` et
   `SERVER_SOURCE_URL`. Il reste à vérifier sur une machine vierge que chaque
   binaire distribué se reconstruit à partir du commit publié, puis à conserver
   une archive de chaque version réellement distribuée ou exploitée. À chaque
   version, remplacer les URL par les nouveaux commits exacts.

3. **Option ultérieure : rendre le périmètre client éligible à SignPath.** Publier les sources des
   configurateurs, viewers et installateurs avec une licence approuvée par
   l'OSI, les scripts de build, les licences des dépendances, la politique de
   signature et une préversion issue de la CI publique. Obtenir l'acceptation
   écrite de SignPath Foundation et vérifier que Windows affiche une signature
   `Valid` pour tous les `.exe` et `.dll`. Indiquer sur la page de téléchargement
   que l'éditeur du certificat est SignPath Foundation. Faire clarifier par
   SignPath le statut de l'API hébergée avant la candidature si son code n'est
   pas publié. Voir `docs/SIGNPATH.md`.

4. **Traiter séparément téléchargements, B2B et B2C.** Conserver les redirections
   de `relaisdesk/downloads/.htaccess` vers les téléchargements vérifiés par l'API.
   Ne pas attendre SignPath pour distribuer des binaires annoncés comme non signés.
   Le médiateur n'est pas
   nécessaire pour vendre exclusivement à un professionnel. Les ventes B2C sont
   déjà ouvertes mais leur dispositif de médiation reste à régulariser (étape 1).
   L'accusé de prise de connaissance configuré dans l'API ne satisfait pas cette
   obligation. Vérifier séparément une commande B2B et une
   commande B2C de bout en bout sur l'environnement de recette.

5. **Maintenir la version contractuelle.** Les versions préparées sont CGV
   `2026-09-21` et essai `2026-09-21-fleet-v2` (paiement en crypto-actifs) ;
   l'annexe RGPD `2026-09-11` reste inchangée et intégrée à la copie
   durable. Toutes les archives antérieures, y compris celles du
   11 septembre, restent inchangées pour les contrats qui les ont acceptées.
   Ne pas publier les nouveaux parcours web avant coordination avec l'API.
   Après de futurs compléments, donner
   un nouveau numéro de version identique dans `relaisdesk/cgv.html`, la
   constante `TERMS_VERSION` de `relaisdesk/app.js`, `publicTermsVersion` dans
   `api/handlers/public_orders.go`, le nom et le contenu du fichier sous
   `api/mailer/legal/`, et `contractualTermsFilename` dans
   `api/mailer/mailer.go`. Rejouer les tests avant déploiement.

## Formalités et contrats à traiter

- Déclarer, si ce n'est pas déjà fait, « RelaisDesk » comme nom commercial et
  l'activité de fourniture du service en ligne via le guichet unique. Une même
  personne ne crée pas une seconde entreprise individuelle ; elle ajoute ou
  modifie les activités de l'EI existante. Si le SaaS devient l'activité
  principale, l'INSEE peut attribuer un autre code APE.
- Depuis le 1er septembre 2026, choisir un canal ou une plateforme agréée pour
  pouvoir recevoir les factures électroniques B2B. La micro-entreprise devra
  également être prête à les émettre électroniquement au 1er septembre 2027.
- Vérifier la disponibilité de « RelaisDesk » (INPI, noms de domaine et droits
  antérieurs) avant une communication publique importante. Envisager un dépôt
  de marque ; l'usage du nom RustDesk doit rester descriptif et la
  non-affiliation visible.
- Signer et archiver les contrats/DPA avec Stripe, OVH, Oracle Cloud et le
  prestataire SMTP. Confirmer la région Oracle réellement utilisée, les
  transferts hors EEE et les garanties correspondantes.
- Mettre en œuvre et faire valider l'annexe `relaisdesk/sous-traitance-rgpd.html`
  pour les données confiées par les clients professionnels. Tenir le registre
  des traitements et les garanties des sous-traitants à jour.
- Renvoyer à l'ANSSI les rectificatifs du 7 septembre en réponse au dossier
  existant. Conserver les accusés et la décision sur le classement demandé.
- Préparer les notifications CRA applicables au 11 septembre 2026 selon
  `PROCEDURE_INCIDENTS_CRA_RGPD.md`, distinctes des notifications RGPD.
- Vérifier l'adéquation de la RC Pro et envisager une couverture cyber. Prévoir
  une procédure d'incident et de notification des violations de données.
- Faire relire les CGV, la politique de confidentialité, la qualification RGPD
  et la limitation de responsabilité par un avocat français avant le premier
  paiement réel, notamment pour les ventes B2C.

## Exploitation et preuves à conserver

- Conserver pendant dix ans les factures et pièces comptables. Archiver la
  version exacte des CGV et du code source correspondant à chaque version
  vendue.
- Configurer `systemd-journald` pour ne pas conserver les journaux techniques
  ordinaires au-delà de douze mois et installer la règle logrotate fournie.
- Vérifier que les e-mails transactionnels sont réellement remis (SPF, DKIM,
  DMARC) et que l'accusé de rétractation arrive sur une seconde boîte de test.
- Ne jamais inscrire une clé privée, un secret Stripe, un IBAN réel ou des
  données client dans un dépôt public.

## Références officielles

- [Mentions obligatoires d'un site professionnel](https://www.economie.gouv.fr/entreprises/developper-son-entreprise/innover-et-numeriser-son-entreprise/mentions-sur-votre-site-internet-les-obligations-respecter)
- [Règles du commerce électronique B2C](https://www.economie.gouv.fr/dgccrf/les-fiches-pratiques/e-commerce-les-regles-entre-professionnels-et-consommateurs)
- [Obligations relatives au médiateur de la consommation](https://www.economie.gouv.fr/mediation-conso/vous-etes-un-professionnel/vos-principales-obligations-0)
- [Information et transparence RGPD](https://www.cnil.fr/fr/conformite-rgpd-information-des-personnes-et-transparence)
- [Durées de conservation des données](https://www.cnil.fr/fr/passer-laction/les-durees-de-conservation-des-donnees)
- [Texte de la GNU AGPLv3](https://www.gnu.org/licenses/agpl-3.0.html)
- [Conditions SignPath Foundation pour les projets open source](https://signpath.org/terms)
