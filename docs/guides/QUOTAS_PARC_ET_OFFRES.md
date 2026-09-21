# Quotas de la console de parc — 10 septembre 2026

API déployée en production sur Oracle le 11 septembre 2026 à 00 h 30 (heure de Paris). Les quotas publics, la santé de l'API et l'intégrité SQLite ont été vérifiés. Aucun déploiement OVH ou GitHub réalisé ; le contrôle du site OVH reste non validé (403/404 constatés avant la bascule). Les prix et les limites de techniciens simultanés sont inchangés. La grille a été révisée avant sa première publication pour limiter Ultra à 4 500 postes ; les documents contractuels déjà publiés avant le 10 septembre restent intacts. Voir le [compte rendu de déploiement](../rapports/DEPLOIEMENT_API_QUOTAS_2026-09-11.md).

## Grille retenue

| Offre | Techniciens simultanés | Postes enregistrables par licence | Prix mensuel | Prix annuel |
| --- | ---: | ---: | ---: | ---: |
| Starter | 1 | 500 | 24,90 € | 239 € |
| Pro | 5 | 1 000 | 110 € | 1 100 € |
| Ultra / Personnalisé | 10 | 2 000 | 199 € | 1 990 € |
| Ultra | 20 | 2 050 | 339 € | 3 390 € |
| Ultra | 50 | 2 200 | 759 € | 7 590 € |
| Ultra | 100 | 2 450 | 1 259 € | 12 590 € |
| Ultra | 500 | 4 500 | 4 059 € | 40 590 € |

Ultra : `2 000 + 5 × (techniciens - 10)` pour 10 à 499 techniciens ; à 500 techniciens, le quota est arrondi de 4 450 à **4 500 postes maximum**, soit 50 places offertes au dernier palier. Cela correspond à +50 appareils par tranche de 10 techniciens supplémentaires, puis à cet arrondi final. Exemples intermédiaires : 200 techniciens → 2 950 postes ; 490 → 4 400 ; 499 → 4 445. Ce n'est pas une nouvelle option payante.

L'assistance ponctuelle par codes temporaires et les installations du logiciel Technicien restent illimitées en nombre. Leur utilisation reste soumise à la capacité simultanée de la licence. Le quota de parc concerne les postes enregistrés pour l'accès permanent, même hors ligne, et s'applique aussi à l'offre choisie pendant l'essai.

## Comptage et protection contre le dépassement

- Le plafond est propre à chaque licence, pas mutualisé entre les licences d'un compte.
- Un poste actif enregistré compte une place, même hors ligne. Les anciens postes nécessitant un réenregistrement comptent également tant qu'ils ne sont pas supprimés.
- Un code d'installation en attente réserve une place pendant 15 minutes. Un code inutilisé expiré ne compte plus, même si sa fiche reste visible.
- La suppression/révocation d'un poste libère sa place. Elle n'efface pas l'historique commercial.
- L'API réserve la place et crée le code dans la même transaction SQLite immédiate. Les demandes simultanées, même depuis plusieurs connexions à la base, ne peuvent pas dépasser le plafond.
- Les routes client et technicien renvoient HTTP 409 lorsque le quota est atteint. Le client ne peut pas imposer son propre quota.
- La console affiche le total utilisé, les réservations et le disponible. Le compteur est global à la licence, indépendant de la pagination des appareils.
- Aucun appareil existant n'est automatiquement supprimé ou déconnecté en cas de dépassement. Les nouveaux ajouts sont bloqués, y compris la validation d'une invitation en attente après une diminution de capacité. Les connexions existantes restent soumises à la validité de la licence.

Le plan provient de la dernière commande payée, sinon de l'essai associé. Pour les licences anciennes/manuelles sans commande ni essai : capacité ≤ 1 → Starter ; 2 à 9 → Pro ; ≥ 10 → Ultra. Vérifier cette correspondance avant de déployer pour des licences manuelles atypiques. Modifier seulement `max_connections` sur une licence payée ne remplace pas son plan commercial.

## Comparaison concurrentielle

Relevé du 10 septembre 2026, capacités incluses hors extensions ou contrats négociés :

| Concurrent | Offre | Appareils gérés inclus |
| --- | --- | ---: |
| AnyDesk | Solo | 100 |
| AnyDesk | Standard | 500 |
| AnyDesk | Advanced | 1 000 |
| AnyDesk | Ultimate | 2 000 |
| TeamViewer | Business | 200 |
| TeamViewer | Premium | 300 |
| TeamViewer | Corporate | 500 |

AnyDesk affiche ces quatre plafonds dans son [comparatif officiel](https://anydesk.com/en/pricing). Le Starter RelaisDesk rejoint donc Standard sur ce seul critère ; Pro rejoint Advanced et Ultra commence au niveau d'Ultimate. AnyDesk propose aussi des extensions : ces nombres ne sont pas nécessairement les plafonds absolus du produit.

TeamViewer indique 200, 300 et 500 appareils dans sa [présentation officielle des licences](https://www.teamviewer.com/apac/global/support/knowledge-base/teamviewer-classic/licensing/purchase/licenses-overview/?language-switched=true). Le Starter RelaisDesk atteint ainsi le nombre inclus dans Corporate. Cela ne rend pas les offres équivalentes : utilisateurs, canaux, sessions, plateformes, intégrations, déploiement et support diffèrent. Un canal TeamViewer ne correspond pas nécessairement à une seule session distante.

Pas d'affirmation de type « le moins cher du marché » : les prix des concurrents dépendent notamment du pays, des promotions, des taxes, des options et de la facturation annuelle. Le nombre d'appareils n'est qu'un critère de comparaison.

### Arbitrages de notre propre grille

Cinq Starter coûtent 124,50 €/mois (1 195 €/an) et donnent 2 500 places réparties entre cinq licences. Pro coûte 110 €/mois (1 100 €/an), soit 14,50 €/mois de moins, mais donne 1 000 places sous une même licence utilisable par cinq techniciens simultanés. Pro est donc moins cher pour une équipe, mais pas le meilleur choix si son seul objectif est de maximiser le nombre de fiches de parc. Ce compromis respecte les plafonds demandés.

Les capacités Ultra de 10 à 500 restent toutes moins chères que la combinaison Starter/Pro la moins coûteuse à capacité simultanée égale ou supérieure. Ce calcul porte sur les techniciens, pas sur une égalité de quotas de parc.

## Vérifications et publication ultérieure

Tests locaux : limites Starter/Pro/Ultra, réservations, expiration, suppression, appartenance à la licence, concurrence, essais, réduction de capacité sans couper les appareils existants, réponses HTTP, catalogue public, affichage du calculateur et conservation des anciennes CGV.

Commandes de validation depuis la racine (Go conforme aux `go.mod` requis) :

```text
go -C database test ./...
go -C api test ./...
node scripts/pricing-checks.mjs
node scripts/legal-checks.mjs
node scripts/trial-contract-checks.mjs
```

Publication à coordonner, pas simplement une mise à jour HTML :

1. Vérifier les contrats déjà acceptés et les éventuels parcs dépassant les nouveaux plafonds. Les archives contractuelles restent intactes, mais le contrôle technique s'appliquera à toutes les licences une fois la nouvelle API démarrée. Ne pas l'activer sur un contrat promettant un parc illimité sans avoir traité la transition avec le client.
2. Tester les binaires/API issus des sources locales. Les fichiers `database/`, `api/handlers/` et `api/mailer/` modifiés doivent être inclus dans la compilation de l'API. Aucun changement du fork RustDesk, des exécutables clients, du VPN ni des mises à jour système n'est nécessaire pour ces quotas.
3. Prévoir une publication coordonnée de l'API et du site : nouvelles CGV `2026-09-10`, essai `2026-09-10-fleet-v1`. Une ancienne page ouverte doit être rechargée avant une nouvelle commande ; son ancienne version contractuelle sera rejetée par la nouvelle API. Les documents des commandes déjà acceptées conservent leur version historique.
4. Transférer sur OVH, depuis le dossier de référence `relaisdesk/`, les fichiers suivants en conservant l'arborescence : `index.html`, `app.js`, `i18n.js`, `cgv.html`, `client/index.html`, `client/app.js`, `essai/index.html`, `essai/app.js`, `essai/conditions.html`. Aucun ZIP nécessaire. Les références de scripts ont été versionnées pour éviter l'ancien cache d'une semaine.
5. Tester une nouvelle commande/inscription, le résumé de quota, l'ajout d'un appareil, sa suppression et le rafraîchissement du compteur, sans générer artificiellement des milliers d'appareils dans la base de production.

Les limites commerciales ci-dessus ne sont pas une certification de capacité du VPS Oracle. Le nombre d'appareils enregistrés n'est pas le nombre d'appareils actifs, mais leurs heartbeats et les connexions relayées consomment des ressources. Une recette de charge reste nécessaire avant d'accueillir de très grands parcs, particulièrement sur une machine avec environ 1 Go de RAM. Aucune supervision permanente n'a été ajoutée.
