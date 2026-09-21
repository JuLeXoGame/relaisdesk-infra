# Application Technicien

L’application Technicien (`installer/configurator`) valide une licence
RelaisDesk, configure RustDesk Community et donne accès à la gestion des codes
Viewer. Elle n’utilise aucun proxy SOCKS5 et aucune fonction RustDesk Server Pro.

## Connexion et configuration

1. Le technicien saisit son identifiant `MP-...` et sa clé `mpsk_...`.
2. L’application appelle `POST /api/v1/activate` en HTTPS.
3. La réponse est rejetée si le serveur, le port, la clé publique ou l’expiration sont invalides.
4. `RustDesk2.toml` est écrit pour joindre directement `api.relaisdesk.fr:21116` et le relais du même hôte.
5. RustDesk est lancé depuis l’installation existante ou depuis le binaire officiel 1.4.9 intégré.

Le raccourci Bureau pointe vers RelaisDesk Technicien et non vers RustDesk ; la
licence est donc contrôlée avant de réappliquer la configuration temporaire.

La configuration ne contient ni `api-server`, ni `proxy-*`, ni identifiants de licence.

## Tableau de bord

Après authentification, un jeton de session API permet de :

- consulter l’état et l’expiration de la licence ;
- créer des codes Viewer temporaires ;
- consulter, copier, révoquer ou supprimer ses propres codes ;
- lancer RustDesk.

Les jetons ne sont pas des clés RustDesk et ne sont jamais écrits dans
`RustDesk2.toml`.

## Stockage local

La session sauvegardée se trouve dans le répertoire applicatif utilisateur avec
des permissions privées. Une déconnexion supprime la session locale et tente de
révoquer le jeton côté API.

## Limite Community

L’expiration d’une licence bloque la connexion à l’application et la création de
nouveaux codes. Elle ne retire pas à distance une configuration RustDesk déjà
écrite sur un poste ; cette propriété ne doit pas être présentée comme une
coupure réseau instantanée.
