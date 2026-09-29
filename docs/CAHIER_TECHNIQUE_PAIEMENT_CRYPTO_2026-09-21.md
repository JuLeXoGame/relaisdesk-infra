# Cahier technique — encaissement Bitcoin/XRP — 2026-09-21

Encaissement direct des abonnements RelaisDesk en Bitcoin (BTC) et XRP, sur
le compte OKX Europe de l'exploitant. Document de cadrage avant
implémentation : aucun encaissement crypto n'est actif tant que les étapes
de la section 9 ne sont pas réalisées et déployées de façon coordonnée
(API Oracle + pages OVH). Les CGV `2026-09-21` (art. 5) ne proposent la
crypto que « lorsqu'elle est proposée au récapitulatif ».

> Mise à jour du 2026-09-22 : l'encaissement se fait désormais
> uniquement sur le compte OKX de l'exploitant (cours, adresses de dépôt
> et rapprochement par l'API OKX). Il n'y a plus aucun portefeuille
> auto-hébergé : toute la garde et la conversion passent par OKX Europe.

## 1. Décisions structurantes

- Compte unique : les fonds sont encaissés sur le compte OKX Europe Ltd
  de l'exploitant (licence CASP MiCA maltaise, passeport EEE, entité
  européenne, pas d'entité offshore).
- Conversion et SEPA : la conversion éventuelle en euros et les virements
  SEPA vers le compte pro se font depuis ce même compte OKX.
- Oracle uniquement : tout le dispositif crypto tourne sur
  l'infrastructure Oracle (région eu-marseille-1, comme l'API). OVH
  héberge les pages statiques et ne reçoit ni clé API, ni secret, ni
  module de surveillance.
- Clé en lecture seule : l'API OKX utilisée par la surveillance ne peut
  ni trader ni retirer ; elle est restreinte par IP dans le back-office
  OKX et conservée sur l'API Oracle uniquement.

## 2. Architecture

```
Client (page OVH) → API Oracle → devis (cours spot OKX)
                 └→ surveillance (historique des dépôts OKX, clé lecture seule)
                 └→ rapprochement → activation licence + facture
Compte OKX Europe ──conversion éventuelle──► SEPA ──► compte pro
```

## 3. Devis et adresses de dépôt

- Chaque commande crypto reçoit un devis : montant exact en BTC (8
  décimales) ou XRP (6 décimales) au cours spot OKX, adresse de dépôt du
  compte OKX, validité de trente minutes au plus.
- Adresse BTC : l'adresse de dépôt BTC sélectionnée du compte OKX,
  identique pour toutes les commandes ; les dépôts sont distingués par
  le montant et la fenêtre de temps.
- Adresse XRP : l'adresse de dépôt XRP sélectionnée du compte OKX, avec
  son tag/memo de dépôt, identique pour toutes les commandes et à
  reprendre exactement (CGV art. 5) ; sans tag, le dépôt ne peut pas
  être crédité par OKX.
- Garde-fou : avant chaque rapprochement, l'adresse configurée est
  vérifiée contre l'adresse sélectionnée du compte OKX. Une coquille de
  configuration suspend le rapprochement au lieu d'attribuer le dépôt
  d'un tiers.

## 4. Rapprochement des dépôts

- La surveillance interroge périodiquement l'historique des dépôts OKX
  (`state=2`, dépôts crédités uniquement), sans jamais exposer de
  données personnelles de client à OKX.
- Un dépôt règle le premier devis correspondant : même adresse créditée,
  montant supérieur ou égal (comparaison rationnelle exacte : le
  sur-paiement paie, le sous-paiement ne paie jamais), dépôt enregistré
  après le devis (marge d'horloge de cinq minutes) et dans les vingt-quatre
  heures suivant son expiration (un crédit BTC peut arriver après
  l'expiration du devis).
- Le traitement est idempotent : rejouements et dépôts en double
  convergent sans dupliquer licences, factures ni lignes de registre.
- Activation de la licence à réception du dépôt crédité (généralement
  quelques dizaines de minutes pour BTC, quelques minutes pour XRP),
  avec envoi de l'identifiant et de la clé d'accès comme pour les autres
  moyens de paiement.
- Dépôts sans devis (montant insuffisant, hors fenêtre) : journalisés
  pour traitement manuel ; ils ne règlent aucune commande.

## 5. Devis et cours (miroir CGV art. 5)

- Source unique écrite au récapitulatif : taux spot OKX BTC/EUR et XRP/EUR.
- Devis garanti pendant la durée affichée, trente minutes au plus. Seul le
  montant en euros fait foi ; frais réseau à la charge du client.
- Sous-paiement après expiration : complément au cours du jour ou annulation.
  Trop-versé minime lié aux arrondis : acquis sans remboursement ; écart
  significatif : régularisé.
- Remboursement (art. 7) : en euros, à hauteur du montant facturé, par
  virement bancaire, sans réexpédition de crypto-actifs.

## 6. Facturation et registre

Chaque facture crypto mentionne : montant en euros, montant crypto payé,
taux appliqué avec date/heure et source, frais éventuels, référence de
transaction (txid). Registre des paiements tenu par lot : date, montant
crypto, cours euros, txid/adresse expéditrice, facture. Conservation dix
ans avec la facture (politique de confidentialité du 21 septembre 2026).
Devis expirés non payés : trois mois au maximum.

## 7. Sécurité opérationnelle

- Compte OKX durci : mot de passe unique, second facteur, clé API en
  lecture seule restreinte à l'IP sortante de l'API Oracle, aucun droit
  de trading ni de retrait.
- Adresses et tag de dépôt recopiés depuis le back-office OKX puis
  vérifiés par le garde-fou automatique avant chaque rapprochement.
- Secrets OKX dans l'environnement de l'API Oracle uniquement (jamais
  sur OVH, jamais dans le dépôt git) ; rotation en cas de doute.
- Journaliser les dépôts, rapprochements et rejets ; alerter sur
  sous-paiement, dépôt sans devis et écart de cours anormal.

## 8. Conformité

- Fiscalité (EI au BIC) : chaque encaissement est une recette pour sa valeur
  en euros au jour de réception ; la revente ultérieure plus cher dégage
  une plus-value professionnelle. Traitement à valider avec
  l'expert-comptable. TVA : franchise art. 293 B du CGI, inchangée.
- Compte OKX hors de France : déclaration annuelle **formulaire 3916-bis**
  (un par compte).
- Réception directe depuis des portefeuilles quelconques : conserver adresse
  expéditrice et txid, refuser les fonds suspects ou sous sanctions
  internationales, ne jamais convertir pour des tiers.
- Données : adresses, tags et txid liés à un client sont des données
  personnelles (politique de confidentialité du 21 septembre 2026,
  registres publics non effaçables). Aucune donnée personnelle de
  client n'est transmise à OKX : la surveillance n'interroge que
  l'historique des dépôts du compte professionnel de l'exploitant.
- L'essai gratuit et l'option d'encaissement des prestations restent
  exclusivement Stripe ; la crypto ne concerne que les abonnements
  RelaisDesk prépayés.

## 9. Étapes de mise en œuvre

1. Compte OKX Europe de l'exploitant : adresses de dépôt BTC/XRP + tag,
   clé API en lecture seule restreinte par IP.
2. Variables OKX sur l'API Oracle (`OKX_API_*`, `OKX_*_DEPOSIT_*`),
   `CRYPTO_ENABLED=true` après recette.
3. Devis dans l'API (cours OKX, validité, règles de sous/sur-paiement).
4. Récapitulatif web : affichage crypto conditionnel + mentions CGV.
5. Factures et registre des paiements (mentions, conservation dix ans).
6. Tests de bout en bout sur petits montants (BTC puis XRP), procédure
   de remboursement en euros.
7. Validation expert-comptable, 3916-bis, durcissement et supervision.
8. Déploiement coordonné API + transfert OVH des pages modifiées.

## 10. Non-objectifs

Pas de conservation ni de change pour des tiers, pas de secrets sur OVH,
pas de paiement crypto pour l'essai gratuit ni pour l'option prestations,
pas de remboursement en crypto-actifs, pas de portefeuille auto-hébergé.
