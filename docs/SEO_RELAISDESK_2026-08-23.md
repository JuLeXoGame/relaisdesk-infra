# Audit SEO de RelaisDesk

Date de l'audit : 23 août 2026

## Résumé

Le socle SEO du site statique `relaisdesk/` a été contrôlé et corrigé. Les pages publiques possèdent maintenant des titres, descriptions, URL canoniques et balises sémantiques cohérents. Les espaces privés sont marqués `noindex`, le sitemap ne contient que les pages publiques et les données structurées de la page d'accueil sont valides au format JSON.

Le domaine définitif `relaisdesk.fr` n'est volontairement pas encore raccordé à l'hébergement : il s'agit d'un état normal de préproduction et non d'une panne. Les contrôles et corrections de ce rapport portent donc sur la nouvelle version présente dans le dossier `relaisdesk/`, pas sur l'ancienne version actuellement installée chez OVH.

L'ancienne version est disponible à l'adresse technique `relaist.cluster129.hosting.ovh.net`. Cette adresse ne doit pas devenir une URL publique canonique. Aucun résultat la concernant n'a été trouvé lors de la recherche d'indexation effectuée le 23 août 2026. Son certificat HTTPS ne correspondait par ailleurs pas à ce sous-domaine lors du contrôle automatisé. Si cet environnement doit rester accessible, il est recommandé de le protéger par authentification et de lui appliquer `noindex` jusqu'à sa suppression.

## Corrections appliquées

- Titre et description de la page d'accueil raccourcis et alignés sur l'intention de recherche « logiciel de support à distance pour techniciens ».
- Suppression de la balise `meta keywords`, qui n'apporte aucune valeur SEO.
- Harmonisation des métadonnées Open Graph et Twitter, avec dimensions et texte alternatif de l'image.
- Ajout et enrichissement des données structurées `WebSite`, `Organization`, `SoftwareApplication` et `FAQPage`.
- Correspondance exacte entre les questions FAQ visibles et leurs données structurées.
- Un seul titre `h1` par page et hiérarchie des titres corrigée dans la section de téléchargement.
- Liens d'accueil normalisés vers `/` pour éviter de renforcer une variante `/index.html`.
- Ajout de libellés accessibles aux menus, logos, formulaire de vérification de licence et accordéons FAQ.
- Ajout de `noindex, nofollow, noarchive, nosnippet` aux espaces `/admin/` et `/technicien/`.
- Modification de `robots.txt` afin que les robots puissent lire ces balises `noindex`.
- Nettoyage du sitemap : uniquement les quatre URL publiques canoniques, dates `lastmod` actualisées et suppression de `priority` et `changefreq`, ignorés par Google.
- Correction des manifestes PWA : identifiants, portée et dimensions réelles de l'icône.
- Remplacement de l'affirmation non vérifiée « binaires signés » par une formulation factuelle.

## Actions indispensables avant la mise en production

1. Déployer la nouvelle version du dossier `relaisdesk/` sur l'hébergement de production.
2. Faire pointer `relaisdesk.fr` et `www.relaisdesk.fr` vers cet hébergement.
3. Installer le certificat TLS du domaine définitif et vérifier que `https://relaisdesk.fr/` répond avec le statut `200`.
4. Rediriger en `301` toutes les variantes HTTP, `www` et `/index.html` vers leurs URL HTTPS canoniques sur `https://relaisdesk.fr`.
5. Vérifier que `/robots.txt` et `/sitemap.xml` répondent en `200` sans authentification.
6. Déclarer la propriété dans Google Search Console, envoyer `https://relaisdesk.fr/sitemap.xml`, puis demander l'indexation de la page d'accueil.
7. Après mise en ligne, mesurer les Core Web Vitals et tester les données structurées avec les outils Google.

Pour les fichiers de `/downloads/`, la directive `Disallow` limite l'exploration mais ne garantit pas à elle seule l'absence d'indexation de l'URL. Si le serveur le permet, ajouter également l'en-tête HTTP `X-Robots-Tag: noindex` aux binaires téléchargeables.

## Contrôles locaux réussis

- Syntaxe JavaScript des trois applications.
- Syntaxe JSON des trois manifestes.
- Syntaxe XML du sitemap.
- Syntaxe JSON des données structurées.
- Existence des cibles de liens et fragments internes.
- Absence d'identifiants HTML dupliqués sur les six pages.
- Réponses locales `200` pour la page d'accueil, `robots.txt`, `sitemap.xml`, l'espace admin et l'espace technicien.

## Documentation de référence

- [Contrôler l'indexation avec `noindex`](https://developers.google.com/search/docs/crawling-indexing/block-indexing)
- [Créer et soumettre un sitemap](https://developers.google.com/search/docs/crawling-indexing/sitemaps/build-sitemap)
- [Règles de `robots.txt`](https://developers.google.com/search/docs/crawling-indexing/robots/robots_txt)
- [Données structurées `SoftwareApplication`](https://developers.google.com/search/docs/appearance/structured-data/software-app)
