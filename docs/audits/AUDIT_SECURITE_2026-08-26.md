# Audit de sécurité et de fiabilité RelaisDesk — 26 août 2026

## Résultat

La couche RelaisDesk, l'API, la base SQLite, les lanceurs et les deux forks ont
été relus. Les défauts confirmés ont été corrigés sans modifier les choix
commerciaux : une licence technicien continue de limiter uniquement les postes
connectés simultanément, et non le nombre total d'installations.

Cette vérification est locale. Elle ne déploie rien sur OVH ou sur le serveur
de production et ne remplace pas un test d'intrusion externe après déploiement.

## Correctifs appliqués

- Un code viewer est maintenant lié durablement à la première clé publique
  Ed25519 qui demande un jeton réseau. Le même poste peut renouveler ses jetons
  courts ; une autre machine utilisant une copie du code est refusée.
- La migration SQLite ajoute `viewer_codes.network_device_public_key` sans
  invalider les codes existants : leur première demande après mise à jour crée
  la liaison.
- L'annonce de l'identifiant RustDesk doit présenter cette même clé liée ; un
  tiers possédant seulement le code ne peut plus remplacer l'identifiant du
  poste affiché au technicien.
- Les lanceurs technicien et viewer n'acceptent plus
  `RELAISDESK_API_URL` depuis l'environnement du poste. L'API est fixée dans le
  binaire et ne peut être remplacée que lors d'une compilation volontaire.
- Une session Stripe n'est plus renvoyée au client si son identifiant n'a pas
  pu être enregistré. La commande est annulée au lieu de laisser un paiement
  impossible à rapprocher automatiquement.
- La suppression d'un `RustDesk.toml` prétendument corrompu utilise désormais
  une analyse TOML ciblée. Un tableau vide sans rapport avec `key_pair` ne peut
  plus provoquer la suppression d'une configuration valide.
- Les réponses JSON reçues par les deux lanceurs et le tableau de bord natif
  sont bornées à 1 Mio afin d'éviter une consommation mémoire non maîtrisée.
- Les entrées JSON publiques de commande et de statut refusent les valeurs
  supplémentaires après le premier objet.
- La procédure ne lance plus simultanément les conteneurs réseau et d'anciennes
  unités systemd. Les unités de secours ont été durcies : utilisateur dédié,
  autorisation obligatoire, fichier d'environnement requis et protections
  systemd.
- `golang.org/x/image`, `golang.org/x/net` et `golang.org/x/text` ont été mis à
  jour vers des versions corrigées dans les trois applications Fyne.

## Vérifications exécutées

- Tests Go : API, base, intégration, configurateur, viewer, tableau de bord,
  générateur de clés et constructeur Debian réussis.
- `go vet` : réussite sur les huit modules Go.
- `govulncheck` : aucune vulnérabilité détectée dans l'API et les applications
  Fyne après mise à jour.
- `cargo audit` : aucune vulnérabilité RustSec dans les verrous du client et du
  serveur.
- `cargo check --locked` : serveur réussi.
- Tests d'autorisation du serveur : 4 réussis (preuve signée, anti-rejeu,
  quota simultané et réservation concurrente).
- Recherche de secrets : aucun secret Stripe, jeton administrateur réel ou clé
  privée n'a été trouvé dans les sources ; seuls les exemples et l'interface
  de connexion correspondent aux motifs recherchés.

## Risques résiduels et prérequis de production

1. Le verrou RustDesk contient encore des dépendances amont signalées comme
   non maintenues ou potentiellement non sûres par RustSec (33 avertissements
   client, 7 serveur), mais aucune vulnérabilité classée comme telle. Leur
   remplacement nécessite une montée de version coordonnée avec RustDesk.
2. La compilation complète du fork client reste à refaire dans l'environnement
   Windows de build équipé de LLVM/libclang. Le contrôle local s'arrête sur
   cette dépendance système manquante ; le serveur compile correctement.
3. Les clés de licence doivent rester lisibles par l'API pour les fonctions
   d'administration. La base active doit donc rester en `0600`, accessible au
   seul compte de service, et ses sauvegardes doivent rester chiffrées avec une
   clé conservée séparément.
4. Les fichiers locaux historiques contenant un jeton DNS ou une licence
   administrateur doivent être déplacés hors du projet et leurs valeurs
   révoquées si elles ont déjà été exposées. Ils sont ignorés par Git, ce qui
   empêche un nouvel ajout mais ne révoque pas un secret existant.
5. Les exécutables distribués doivent être reconstruits depuis cet état,
   signés, intégrés au manifeste de version signé, puis déployés avec l'API et
   les forks correspondants. Mélanger anciens lanceurs et nouvelle API n'est
   pas une procédure de mise à jour supportée.

## Architecture contrôlée

Le flux RustDesk reste direct : `hbbs` sur 21115/21116 et `hbbr` sur 21117,
sans proxy applicatif. L'API émet des autorisations Ed25519 courtes liées aux
clés d'appareil. `hbbs` contrôle rôle, locataire et quota technicien simultané ;
`hbbr` n'apparie que deux rôles complémentaires du même locataire.
