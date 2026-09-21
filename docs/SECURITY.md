# Securite

## Modele

- Les clients RustDesk utilisent directement le serveur Community `hbbs`/`hbbr`.
- La clé publique `id_ed25519.pub` authentifie le serveur RustDesk.
- Une clé Ed25519 distincte signe des autorisations réseau de cinq minutes.
- Les forks `hbbs`/`hbbr` contrôlent ces autorisations sans rappeler l'API pour
  chaque paquet et sans accéder à la clé privée de signature.
- Aucun proxy SOCKS5, identifiant de licence, code viewer ou jeton n'est écrit
  directement dans `RustDesk2.toml`; seuls des chemins de fichiers privés y
  figurent.

## Authentification

- Les sources intègrent les [correctifs du 12 septembre 2026](CORRECTIFS_SECURITE_2026-09-12.md) : respect de la 2FA sur les parcours de récupération et d'administration, consommation atomique des codes, révocation des sessions après modification des identifiants, et stockage Windows DPAPI. Leur présence dans les sources ne signifie pas qu'ils sont déjà déployés.
- Les clés de licence et codes viewer sont vérifiés uniquement par l'API HTTPS.
- L'URL de l'API des lanceurs distribués est fixée à la compilation : une
  variable d'environnement locale ne peut pas rediriger une clé de licence ou
  un code viewer vers un serveur tiers.
- Chaque message sensible contient une preuve Ed25519 liée à l'action, au
  jeton, à l'heure et à un nonce. Un jeton copié seul est inutilisable.
- Le premier jeton réseau lie définitivement un code viewer à la clé publique
  de son appareil. Les renouvellements et l'annonce de l'identifiant RustDesk
  exigent cette même clé.
- `hbbs` n'autorise qu'un technicien à initier une connexion vers un viewer du
  même locataire. `hbbr` vérifie les deux extrémités avant de les apparier.
- Les clés complètes et codes complets ne sont jamais loggés en clair.
- La clé privée RustDesk `id_ed25519` reste exclusivement sur le serveur ; les
  clients ne reçoivent que `id_ed25519.pub`.
- `RELAISDESK_AUTH_REQUIRED=Y` est obligatoire sur les deux services. Un
  service sans clé d'autorisation valide refuse alors de démarrer.

## Ports publics du serveur

- `443/tcp` : API HTTPS RelaisDesk (`api.relaisdesk.fr`)
- `21115/tcp` : test NAT RustDesk
- `21116/tcp` et `21116/udp` : ID/rendezvous et perforation NAT
- `21117/tcp` : relais RustDesk
- `22/tcp` : SSH
- `80/443/tcp` : HTTP/HTTPS (challenges Let's Encrypt / Web)
- `21118-21119/tcp` restent fermés tant qu'aucun client Web n'est utilisé.
- Les en-têtes d'adresse des WebSockets sont ignorés sauf si l'adresse du
  frontal figure explicitement dans `RELAISDESK_TRUSTED_WS_PROXY_IPS`.

## Protections

- Blocage apres 5 echecs d'authentification.
- Ban de 15 minutes.
- Limite `max_connections` réservée atomiquement pour les appareils
  techniciens actifs par `hbbs`, y compris lors d'inscriptions concurrentes.
- Jetons de cinq minutes renouvelés avant expiration et révocation propagée sans
  rotation de la clé RustDesk générale.
- Canal de santé signé toutes les dix secondes sur les deux extrémités ; le
  technicien comme le viewer coupent leur session si leur propre autorisation
  ne peut plus être confirmée.
- Rotation sans interruption par coexistence temporaire de plusieurs `kid`.
- Les événements Stripe et courriels sont persistés avant traitement, dotés de
  clés d'idempotence et repris après redémarrage ; le lien magique n'est jamais
  stocké en clair dans la file.
- L'isolation de l'espace commercial repose sur un `customer_id` stable et une
  table d'appartenance, pas sur une adresse e-mail mutable.
- Les sauvegardes quotidiennes utilisent un instantané SQLite cohérent,
  vérifié, chiffré et testé en restauration. La clé AES-256 doit être conservée
  séparément hors ligne.
- La production refuse une configuration SMTP dont l'expéditeur n'appartient
  pas au domaine déclaré ou dont le sélecteur DKIM est invalide. SPF, DKIM et
  DMARC sont vérifiables par la commande ponctuelle `relaisdesk-emailcheck`.

## Limites assumées

Sans faire transiter tout le flux P2P par un proxy contrôlé, le serveur ne peut
pas arracher physiquement une connexion directe déjà établie à un client
hostile qui aurait supprimé le canal de santé dans son propre fork. Le client
RelaisDesk fourni applique la coupure, et toute nouvelle connexion ou
réinscription échoue dès l'expiration du jeton. Copier volontairement le jeton,
la clé privée d'appareil et un binaire modifié reste également un partage de
secrets ; le quota limite l'usage concurrent mais aucun logiciel libre ne peut
fournir une DRM absolue sur une machine contrôlée par son utilisateur.
