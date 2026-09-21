# Registre RGPD — essais et abonnements RelaisDesk

Établi le 9 septembre 2026 pour la livraison `2026-09-09-trial-v2`. Document interne, à joindre au registre de l'entreprise et à maintenir ; il ne s'agit pas d'une certification CNIL. Les choix doivent être réévalués au regard de l'exploitation réelle.

Complément du 11 septembre 2026 : le registre et les procédures de la console de parc et de l'accès permanent figurent dans [JURIDIQUE_PARC_ET_ACCES_PERMANENT.md](JURIDIQUE_PARC_ET_ACCES_PERMANENT.md). Les nouveaux contrats préparés portent les versions CGV `2026-09-11` et essai `2026-09-11-fleet-v2` ; les archives des essais déjà acceptés restent inchangées.

## Responsable et périmètre

Julien BELLOT, entrepreneur individuel, Informatique à Domicile 03 / RelaisDesk, SIRET 94074710800014, 2 Chemin de Lonzais, 03170 Bizeneuille. Contact pour les droits : contact@relaisdesk.fr, formulaire du site ou courrier. Accès opérationnel réservé à l'exploitant et aux personnes expressément habilitées.

Personnes : demandeurs d'essai, clients particuliers et représentants de clients professionnels. Origines : formulaires, confirmation d'e-mail, Stripe et événements du service. Aucune prospection ajoutée par ce parcours.

## Traitements

| Finalité | Données et base juridique | Durée / sort des données |
| --- | --- | --- |
| Préparer la souscription et exécuter l'abonnement | Identité et adresse de facturation, e-mail, statut déclaré, offre, capacité, montant, périodicité, preuve d'acceptation, identifiants Stripe et licence ; mesures précontractuelles et contrat, art. 6(1)(b) | Demandes abandonnées sans contrat : 30 jours. Contrats et preuves : pendant la relation, puis archivage restreint selon la politique générale et les délais de preuve applicables. |
| Factures et obligations comptables | Coordonnées de facturation, montants et références ; obligation légale, art. 6(1)(c) | Pièces comptables : politique générale de conservation de 10 ans ; pas de suppression d'une facture pour réinitialiser un essai. |
| Prévenir les répétitions d'essai | Marqueurs HMAC séparés de l'e-mail, du compte et de l'empreinte de carte Stripe, associés à la demande ; intérêt légitime, art. 6(1)(f) | Pendant l'abonnement puis au maximum 3 ans après sa fin ; purge automatique. La clé HMAC est sauvegardée et ne sert à aucune autre finalité. |
| Rétractations et arrêt des renouvellements | Référence du contrat, identité déclarée, e-mail, demande et horodatage, suites techniques et traces d'envoi ; contrat et obligations légales | Preuves de rétractation d'essai : 5 ans après la fin du contrat ; purge automatique, sauf conservation distincte justifiée par un litige. |
| Information annuelle de reconduction | Contrat, échéance, prix, texte exact du rappel et date d'acceptation SMTP ; obligation légale pour les contrats concernés | 5 ans après la fin du contrat ; purge automatique. La trace SMTP n'est pas une preuve de lecture ni une garantie de réception. |

L'historique de licence conservé pour une finalité légitime peut encore rendre un compte inéligible après purge des marqueurs. Ne pas prolonger l'ensemble des données contractuelles uniquement pour une exclusion perpétuelle du gratuit. Les contrats clôturés doivent faire l'objet de la revue d'archivage générale : cette livraison n'automatise pas la suppression de toutes les licences historiques.

## Mise en balance de l'intérêt légitime anti-abus

- Intérêt : éviter la consommation répétée d'un service payant grâce à de nouveaux comptes, après annulation ou non-renouvellement. Un contrôle par e-mail seul ne suffit pas.
- Nécessité/minimisation : trois marqueurs ciblés, pas de numéro de carte, cryptogramme, IBAN, solde, relevé bancaire, profil de solvabilité, empreinte de navigateur ou rapprochement avec des tiers. Le SIRET public n'est pas une preuve d'identité et ne déclenche pas une exclusion automatique.
- Conséquences : refus du nouvel essai, sans création d'un abonnement facturable. Aucun client n'est publiquement désigné comme fraudeur ; le résultat signifie « essai non accordé », pas « fraude établie ».
- Risques : carte partagée, carte virtuelle, changement de carte et faux positifs. L'empreinte ne prouve pas l'identité d'une personne. Les HMAC sont des données pseudonymisées, non anonymes.
- Garanties : information avant souscription, clé séparée, contrôle d'accès, absence de données brutes dans les journaux, possibilité d'examen humain et d'opposition, limitation de conservation. Aucun refus ne doit conduire à exiger une copie de carte ou un relevé bancaire pour le contester.
- Durée : les 3 ans sont une hypothèse initiale couvrant plusieurs cycles annuels et retours après résiliation, **pas une durée imposée ni approuvée par la CNIL**. Aucun historique statistique d'abus ne permet encore d'en établir la nécessité. L'exploitant doit réévaluer cette durée après les premiers mois, puis au moins annuellement, et la réduire si l'intérêt n'est pas démontré. Consigner cette revue ici.

## Destinataires, sécurité et transferts

Oracle héberge l'API et SQLite ; OVH héberge le site et assure le service de messagerie utilisé ; Stripe traite la vérification de carte et la facturation. Les marqueurs HMAC ne sont pas envoyés au site vitrine ni à un fichier public. L'hébergement indiqué par l'exploitant et les accords de sous-traitance/transfert des fournisseurs restent à vérifier dans les contrats effectifs : ne pas déduire qu'un serveur Oracle en France exclut tout transfert lié à Stripe ou au support des fournisseurs.

SQLite et sauvegardes privées : droits restreints, pas de mélange TEST/LIVE, pas de secrets dans les archives OVH ou GitHub. L'API lie désormais la base au mode Stripe choisi. Aucun nouveau cookie publicitaire n'est ajouté.

## Exercice des droits et contestation

1. Enregistrer la demande reçue par formulaire, courriel ou courrier ; vérifier proportionnellement l'identité (session du compte ou confirmation à son adresse), sans demander de carte bancaire.
2. Identifier les données concernées et expliquer les marqueurs, l'origine Stripe et les limites. Ne pas divulguer la clé HMAC, d'autres comptes ou les données d'un autre titulaire de carte.
3. Examiner humainement le cas (notamment carte d'entreprise partagée), rectifier toute erreur et motiver la décision. Une solution commerciale peut être accordée manuellement ; ne jamais effacer à l'aveugle les marqueurs communs ni créer un abonnement facturable sans nouvel accord.
4. Répondre dans le délai RGPD applicable, normalement un mois ; expliquer une éventuelle prolongation motivée et les recours, dont la réclamation à la CNIL.
5. Lors d'un effacement, distinguer données opérationnelles, opposition anti-abus et pièces à conserver légalement ; tracer le fondement de toute conservation résiduelle et appliquer la politique de sauvegarde.

Sources de cadrage : [intérêt légitime — CNIL](https://www.cnil.fr/fr/les-bases-legales/interet-legitime), [durées de conservation — CNIL](https://www.cnil.fr/fr/passer-laction/les-durees-de-conservation-des-donnees), [RGPD — CNIL](https://www.cnil.fr/fr/reglement-europeen-protection-donnees).
