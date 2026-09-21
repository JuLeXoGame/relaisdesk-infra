# Console de parc et accès permanent — cadre juridique et mise en œuvre

Version préparée le 11 septembre 2026. Document interne de Julien BELLOT EI / RelaisDesk, SIRET 94074710800014. Ces modifications sont locales : aucune publication OVH, aucun déploiement Oracle ni aucune démarche externe ne sont réalisés par cette mise à jour.

## 1. Documents et portée

Les CGV et leur annexe RGPD passent à la version `2026-09-11` ; les conditions d'essai passent à `2026-09-11-fleet-v2`. Prix, essai de 30 jours, prélèvement automatique annoncé, annulation, garanties et quotas ne changent pas : Starter 500 postes, Pro 1 000, Ultra de 2 000 à 4 500 postes.

Les ajouts couvrent le parc, les connexions sans validation interactive, les habilitations, le retrait d'autorisation, les données collectées, leurs durées et les limites des suppressions. L'autorisation de contrôler un poste n'est pas, à elle seule, une base légale RGPD. Les responsabilités doivent correspondre au traitement réel, conformément aux [recommandations de la CNIL sur les contrats de sous-traitance](https://www.cnil.fr/fr/responsable-de-traitement-et-sous-traitant-6-bonnes-pratiques-pour-respecter-les-donnees).

Les fichiers historiques de CGV et d'essai dans `api/mailer/legal/` restent intacts. L'API conserve leur routage par version pour les commandes, confirmations et abonnements existants. Aucun consentement nouveau n'est inventé. Pour un client existant dont le contrat ne couvre pas ces traitements, communiquer le complément et recueillir l'accord ou les instructions documentées nécessaires avant d'étendre son usage ; une simple mise à jour de page ne remplace pas cette démarche.

## 2. Registre RGPD complémentaire

Responsable des traitements propres : Julien BELLOT EI. Contact : contact@relaisdesk.fr, formulaire du site, ou 2 Chemin de Lonzais, 03170 Bizeneuille. Pour les données de ses utilisateurs confiées par un professionnel, RelaisDesk est sous-traitant ou sous-traitant ultérieur ; le Client documente la chaîne d'autorisation.

| Traitement | Données, personnes et finalité | Conservation et mise en œuvre |
| --- | --- | --- |
| Inventaire de parc, sur instruction du Client | Identifiants de poste et RustDesk, compte/licence, nom d'hôte, OS, alias, notes, dates. Techniciens, utilisateurs réguliers, salariés et clients éventuellement identifiables. Organiser la maintenance, sans mesure de productivité. | Pendant le service, puis au plus 30 jours après sa fin effective pour restitution, ou suppression antérieure légalement possible. Revue manuelle, pas de purge automatique des fiches dans l'API actuelle. |
| Enrôlement et autorisation du poste | Empreinte du code d'installation, clé publique du poste, état d'enrôlement. Code brut communiqué à la création ; contrôle de preuve de possession. | Code valable 15 minutes ; un code expiré ne réserve plus de quota. Fiche résiduelle inutilisée à effacer manuellement au plus tard 30 jours après expiration. |
| Présence et anti-rejeu | Dernière IP et présence, statut annoncé, dates, nonces et expiration. Assurer la sécurité et présenter le dernier état connu, pas un journal exhaustif de sessions. | Dernière IP et présence remplacées par les mises à jour ; même durée que la fiche. Les nonces expirés sont nettoyés lors des preuves suivantes ; en l'absence d'activité, nettoyage manuel lors de la revue. |
| Gestion des droits et incidents | Demande, périmètre, instruction, référence et résultat d'effacement, éventuel incident isolé. | Preuve minimale à accès restreint ; durée justifiée au cas par cas selon la politique générale, pas de copie intégrale du parc à conserver comme preuve. |

Origines : formulaires du Client et messages authentifiés des postes. Destinataires : exploitant, membres et techniciens autorisés selon le compte ou la licence ; OCI pour l'API, la base et le réseau, OVHcloud pour le site et la messagerie. Stripe n'a pas à recevoir les notes ou l'inventaire pour encaisser. Google Drive n'est pas déclaré comme stockage actif : sa connexion et les garanties contractuelles doivent être vérifiées avant toute utilisation réelle.

Pour les données du Client nécessaires au service : contrat ; pour la sécurité propre du réseau : intérêt légitime à protéger les utilisateurs. Le Client professionnel détermine et documente la base légale applicable à ses salariés et autres utilisateurs, les destinataires et leur information. L'hypothèse d'un consentement libre des salariés ne doit pas être présumée. Examiner la nécessité d'une analyse d'impact selon le contexte et les risques, sans considérer l'achat du logiciel comme une autorisation de surveillance.

Mesures vérifiées dans le code : contrôle par compte/licence, code haché à durée limitée, preuve Ed25519 et anti-rejeu, jetons temporaires, séparation des secrets locaux et de l'API. La clé privée du poste et le mot de passe permanent configuré par le Viewer ne font pas partie de l'inventaire API. Le chiffrement des sessions n'est pas un chiffrement de bout en bout des notes et métadonnées.

## 3. Modèle d'autorisation à compléter avant installation

À adapter à l'organisation réelle, à faire valider par la personne habilitée et à conserver dans un espace documentaire protégé, pas dans les notes publiques du poste. Ne pas recueillir de mot de passe dans ce document.

- Organisme ou propriétaire : [nom et coordonnées].
- Signataire, qualité et pouvoir d'autoriser : [nom, fonction, périmètre].
- Prestataire/équipe habilitée et contact : [identité, personnes ou rôles limités].
- Postes concernés : [liste d'identifiants utiles, sans données superflues].
- Finalité : [maintenance, dépannage, opérations expressément autorisées].
- Période et plages horaires autorisées : [début, fin, restrictions].
- Fonctions permises : [écran, saisie, fichiers, presse-papiers, redémarrage… selon le besoin réel].
- Information remise aux utilisateurs : [date, document, canal, contact droits].
- Base légale RGPD et, si nécessaire, obligations de consultation accomplies : [référence documentée].
- Retrait ou changement d'habilitation : [contact, procédure, traitement de l'urgence].

« J'autorise les seuls accès décrits ci-dessus. Je suis informé que le service permanent peut fonctionner sans interface Viewer ouverte et permettre une connexion sans validation interactive à chaque session. Cette autorisation ne couvre ni une surveillance clandestine, ni des opérations étrangères à sa finalité. Je peux demander son retrait dans les conditions indiquées. »

Date, identité et validation du signataire : [à compléter]. Le Client vérifie les pouvoirs du signataire et l'information des utilisateurs ; la signature d'un technicien seul ne prouve pas l'autorisation du propriétaire.

## 4. Retrait d'accès et effacement : procédure à appliquer

Responsable opérationnel : l'exploitant. Désigner un suppléant si nécessaire. Mettre en place une revue au moins hebdomadaire, sans attendre le dernier jour des délais ci-dessous. Aucun nouvel automate ni aucune suppression de production ne sont activés par cette livraison.

1. Vérifier l'identité et le pouvoir du demandeur, le compte, la licence et les postes exacts. Pour une demande concernant les données confiées par un professionnel, suivre ses instructions et les obligations RGPD applicables.
2. En cas de retrait d'accès : supprimer le poste depuis la console ou par l'opération autorisée correspondante. Vérifier le refus des nouvelles autorisations. Les jetons de poste sont bornés à cinq minutes par `issueDeviceNetworkToken`, mais cette borne ne prouve pas à elle seule la coupure instantanée d'une session : contrôler aussi le comportement du client et du serveur.
3. En urgence, faire interrompre la session et désactiver localement les services par une personne habilitée. L'opération `--unenroll` du Viewer Windows retire le service de parc et ses fichiers dédiés ; elle ne constitue pas une désinstallation complète de RustDesk et ne supprime pas tous ses réglages. Vérifier les mots de passe et autres accès restants.
4. Lors de chaque revue, identifier les fins effectives de licence (expiration sans renouvellement ou résiliation définitive), les codes inutilisés expirés et les demandes d'effacement. Ne jamais supprimer un poste seulement parce qu'il est hors ligne. Vérifier un éventuel renouvellement avant effacement.
5. Restituer les données sur demande dans un format utilisable. Effacer les fiches du parc au plus tard 30 jours après la fin effective du service et les fiches d'enrôlement inutilisées au plus tard 30 jours après expiration du code. En l'absence d'interface utilisable après expiration, l'exploitant fait réaliser une intervention de base strictement ciblée, testée sur copie, avec vérification des clés étrangères. Pas de suppression globale des comptes ou licences.
6. Contrôler l'absence des fiches et des nonces associés. Nettoyer aussi les nonces déjà expirés qui resteraient sans activité. Conserver une preuve minimale datée du résultat ; traiter séparément les fiches d'intervention, factures et archives contractuelles.
7. Si une conservation exceptionnelle est juridiquement nécessaire, isoler les seules données utiles, restreindre l'accès et documenter le motif et l'échéance ; ne pas invoquer un litige pour conserver tout le parc.

Cette politique de 30 jours est un choix de conservation interne, pas un délai légal universel. La procédure manuelle doit être réellement organisée avant publication de ces engagements. Si les volumes ne permettent plus de respecter ces délais, développer et tester une purge dédiée avant de poursuivre selon cette politique. La [CNIL rappelle que la durée dépend de la finalité](https://www.cnil.fr/fr/passer-laction/les-durees-de-conservation-des-donnees).

### Sauvegardes et restauration

Inventorier les sauvegardes locales, copies distantes et snapshots Oracle ; noter pour chacun la durée, la rotation, les accès et le responsable. Vérifier ces paramètres réels avant publication : ce document ne configure ni Google Drive ni une rotation de snapshots. Ne pas promettre une suppression immédiate de toutes les sauvegardes.

Tenir un relevé minimal protégé des effacements et retraits à réappliquer tant qu'une sauvegarde susceptible de les contenir existe. Après restauration, garder le service fermé aux accès clients, réappliquer les retraits/effacements, vérifier les accès et tester l'intégrité avant réouverture. Purger aussi ce relevé lorsqu'il n'est plus nécessaire.

## 5. Publication coordonnée — ne pas transférer le site seul

Les fichiers à transférer ultérieurement sur OVH, ensemble et dans leur arborescence :

- `relaisdesk/cgv.html`, `politique-confidentialite.html`, `sous-traitance-rgpd.html`, `logiciel-libre.html` ;
- `relaisdesk/index.html`, `app.js`, `i18n.js`, `styles.css` ;
- `relaisdesk/client/index.html` et `client/app.js` ;
- `relaisdesk/essai/index.html`, `essai/app.js` et `essai/conditions.html`.

La nouvelle API doit être construite avec les modifications de `api/mailer/` et `database/trials.go`. L'API actuellement déployée attend les versions du 10 septembre : publier seulement les nouveaux parcours web provoquerait des refus d'acceptation contractuelle. Prévoir une courte maintenance des commandes et essais, déployer les deux côtés, puis vérifier les versions exposées par `/api/v1/public/trials`, les cases non précochées, les pièces jointes et une commande de test avant réouverture. Le script `deploy_api_quota_update.sh` est spécifique à l'ancien déploiement de quotas : ne pas le réutiliser tel quel pour ce changement juridique.

Ne pas changer rétroactivement les versions enregistrées dans la base. Les contrats en cours et anciennes demandes continuent avec leur version archivée. Les opérations d'envoi réel et de déploiement nécessitent une validation distincte ; aucun achat Stripe réel n'est nécessaire aux tests unitaires.

Les binaires 1.0.2 et leur manifeste ne sont pas modifiés par cette livraison. Ils restent à tester avant diffusion. La page Logiciel libre distingue la source du moteur précédent et le moteur e34c8bab des paquets préparés. Vérifier l'accès public anonyme aux sources complètes des moteurs, lanceurs, dépendances modifiées et scripts correspondant à chaque paquet ; le lien vers le seul moteur ne suffit pas. La signature Ed25519 n'est pas Authenticode et SignPath n'est pas une condition de distribution.

## 6. Points qui nécessitent une démarche réelle

- **Médiateur de la consommation :** il reste à contractualiser un dispositif compétent, puis publier ses coordonnées effectives. Le signalement de son absence ne remplace pas l'obligation. Les ventes B2C ne sont pas désactivées par cette livraison. Voir les [obligations officielles](https://www.economie.gouv.fr/mediation-conso/vous-etes-un-professionnel/vos-principales-obligations-0).
- Appliquer effectivement la revue d'effacement, contrôler les sauvegardes et recueillir les autorisations : écrire les clauses ne réalise pas ces opérations.
- Les dossiers ANSSI déjà envoyés ne sont ni modifiés ni renvoyés ici. Cette mise à jour de textes ne prouve pas qu'une déclaration couvre une nouvelle version du logiciel ; une comparaison avec le dossier réellement transmis reste nécessaire si le périmètre technique a changé.
- Ces adaptations documentaires ne valent pas certification ni validation exhaustive par un juriste. Faire examiner les cas particuliers, notamment l'usage en entreprise et la chaîne de sous-traitance.
