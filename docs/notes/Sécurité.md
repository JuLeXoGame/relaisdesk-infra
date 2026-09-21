# Modèle de sécurité actuel

RelaisDesk s’appuie sur RustDesk Community en accès direct à `hbbs`/`hbbr`.
Il n’existe aucun proxy SOCKS5 d’autorisation. La clé publique RustDesk permet
de vérifier le serveur ; elle n’est pas un secret de licence.

## Contrôles appliqués

- API HTTPS obligatoire en production, délais HTTP et limites de corps.
- Limites de débit séparées pour les authentifications sensibles.
- Sessions administrateur hachées en base et jetons aléatoires.
- Sessions Technicien contrôlées en base avec expiration.
- Vérification Stripe signée, fenêtre anti-rejeu et traitement atomique.
- SMTP uniquement en TLS direct 465 ou STARTTLS 587, TLS 1.2 minimum.
- Base SQLite, factures, exports et configurations locales en permissions privées.
- Codes Viewer aléatoires, temporaires, liés à une licence active et à un seul ID.
- Réponses RustDesk validées avant écriture du TOML.
- Téléchargement Linux RustDesk limité en taille et vérifié par SHA-256.
- CSP stricte dans les espaces Admin et Technicien, sans scripts inline.
- Répertoires applicatifs serveur non modifiables par l’utilisateur du service.

## Secrets à protéger

- `ADMIN_TOKEN`, secrets Stripe, SMTP et DNS ;
- clés de licence client ;
- clé privée `hbbs` ;
- base de données de production et factures.

Ces valeurs ne doivent pas être conservées dans le dépôt, les journaux, les
captures d’écran ou les artefacts téléchargeables. Une valeur ayant été exposée
doit être révoquée et remplacée.

## Risques résiduels

1. Une configuration RustDesk Community déjà écrite n’est pas révoquée par
   l’expiration ultérieure d’une licence ou d’un code.
2. RustDesk reste un projet amont volumineux avec des dépendances
   multi-plateformes ; chaque nouvelle version doit être auditée et reconstruite.
3. Les binaires Windows non signés déclenchent SmartScreen et sont plus faciles
   à substituer lors d’une distribution non maîtrisée.
4. La préproduction OVH doit rester `noindex` et disposer d’un certificat TLS
   correspondant exactement à son nom d’hôte.
