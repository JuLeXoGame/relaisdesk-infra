# Cahier technique — encaissement Bitcoin/XRP — 2026-09-21

Encaissement direct des abonnements RelaisDesk en Bitcoin (BTC) et XRP, avec
conservation sur matériel Ledger détenu par l'exploitant. Document de cadrage
avant implémentation : aucun encaissement crypto n'est actif tant que les
étapes de la section 11 ne sont pas réalisées et déployées de façon
coordonnée (API Oracle + pages OVH). Les CGV `2026-09-21` (art. 5) ne
proposent la crypto que « lorsqu'elle est proposée au récapitulatif ».

## 1. Décisions structurantes

- Garde propre : les fonds sont conservés sur le Ledger de l'exploitant.
  Aucune prestation de conservation ou de change pour le compte de tiers.
- Rampe de sortie : OKX Europe Ltd (licence CASP MiCA maltaise, passeport
  EEE) pour la conversion éventuelle en euros et les virements SEPA. Le
  compte est rattaché à l'entité européenne, pas à une entité offshore.
- Oracle uniquement : tout le dispositif crypto tourne sur
  l'infrastructure Oracle (région eu-marseille-1, comme l'API). OVH
  héberge les pages statiques et ne reçoit ni clé privée, ni graine,
  ni xpub, ni module de surveillance.
- Aucune clé privée sur un serveur : BTC en surveillance seule (xpub),
  XRP en surveillance d'adresse. La dépense exige le boîtier physique.

## 2. Architecture

```
Client (page OVH) → API Oracle → module BTC (BTCPay, xpub) ──► Ledger (hors ligne)
                 └→ module XRP (adresse + tags) ──► Ledger (hors ligne)
                 └→ webhooks internes → activation licence + facture
Ledger ──vente manuelle──► OKX Europe ──► SEPA ──► compte pro
```

## 3. Module Bitcoin : BTCPay Server auto-hébergé

- BTCPay Server sur Oracle, portefeuille configuré avec la **xpub du
  Ledger** (surveillance seule). Adresse neuve par facture.
- Suivi des confirmations : activation de la licence après le nombre de
  confirmations affiché au récapitulatif (au moins une, CGV art. 5).
- Webhook signé de BTCPay vers l'API : rapprochement commande/facture,
  envoi de l'identifiant et de la clé d'accès comme pour les autres
  moyens de paiement.
- BTCPay et l'API restent sur le réseau privé Oracle ; seul le récapitulatif
  (montant, adresse, délai) est exposé au client via l'API publique.

## 4. Module XRP : adresse + Destination Tags

BTCPay ne gère pas XRP nativement : module dédié sur Oracle.

- Une adresse XRP Ledger professionnelle, dont la clé est sur le Ledger.
- Un **Destination Tag unique par facture**, communiqué au client et à
  reprendre exactement (CGV art. 5) ; sans tag, attribution impossible.
- Surveillance des paiements validés par le registre (adresse + tag +
  montant) puis webhook interne vers l'API pour activation.
- Maintenir la réserve en XRP immobilisée par le compte ; tester avec de
  petits montants avant ouverture.

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

- Graine Ledger sauvegardée sur deux supports distincts hors ligne (jamais
  photo ni cloud) ; firmware via Ledger Live uniquement.
- Adresses de réception vérifiées sur l'écran du boîtier avant communication.
- Prévoir perte, vol et transmission (décès) : procédure écrite, personnes
  désignées, sans exposer la graine.
- Journaliser les webhooks et rapprochements ; alerter sur sous-paiement,
  tag manquant et écart de cours anormal.

## 8. Conformité

- Fiscalité (EI au BIC) : chaque encaissement est une recette pour sa valeur
  en euros au jour de réception ; la revente ultérieure plus cher dégage
  une plus-value professionnelle. Traitement à valider avec
  l'expert-comptable. TVA : franchise art. 293 B du CGI, inchangée.
- Compte OKX hors de France : déclaration annuelle **formulaire 3916-bis**
  (un par compte). Le Ledger auto-hébergé n'y entre pas.
- Réception directe depuis des portefeuilles quelconques : conserver adresse
  expéditrice et txid, refuser les fonds suspects ou sous sanctions
  internationales, ne jamais convertir pour des tiers.
- Données : adresses, tags et txid liés à un client sont des données
  personnelles (politique de confidentialité du 21 septembre 2026,
  registres publics non effaçables).
- L'essai gratuit et l'option d'encaissement des prestations restent
  exclusivement Stripe ; la crypto ne concerne que les abonnements
  RelaisDesk prépayés.

## 9. Étapes de mise en œuvre

1. BTCPay Server sur Oracle (xpub Ledger, webhooks signés vers l'API).
2. Module XRP sur Oracle (adresse, tags uniques, surveillance, webhooks).
3. Devis dans l'API (cours OKX, validité, règles de sous/sur-paiement).
4. Récapitulatif web : affichage crypto conditionnel + mentions CGV.
5. Factures et registre des paiements (mentions, conservation dix ans).
6. Tests de bout en bout sur petits montants (BTC puis XRP), procédure
   de remboursement en euros.
7. Validation expert-comptable, 3916-bis, durcissement et supervision.
8. Déploiement coordonné API + transfert OVH des pages modifiées.

## 10. Non-objectifs

Pas de conservation ni de change pour des tiers, pas de clés sur OVH, pas
de paiement crypto pour l'essai gratuit ni pour l'option prestations, pas
de remboursement en crypto-actifs.
