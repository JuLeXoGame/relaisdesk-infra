# Audit SEO RelaisDesk — 24 août 2026

## Résultat

Le socle SEO de la nouvelle version locale `relaisdesk/` est propre et prêt à
être déployé. La version actuellement accessible sur
`relaist.cluster129.hosting.ovh.net` est bien l'ancienne version, conformément à
l'état de préproduction annoncé ; elle n'a pas été écrasée par cet audit.

## Nouvelle version locale

- Page d'accueil : titre de 57 caractères, description de 145 caractères, une
  seule balise `h1` et canonique `https://relaisdesk.fr/`.
- Pages légales : titre, description, canonique et un seul `h1`.
- Espaces Admin et Technicien : `noindex, nofollow, noarchive, nosnippet`, hors
  sitemap.
- Données structurées `Organization`, `WebSite`, `SoftwareApplication` et
  `FAQPage` valides ; version logicielle alignée sur `1.0.0`.
- Sitemap limité aux quatre pages publiques, dates `lastmod` au 24 août 2026.
- `robots.txt` laisse lire les balises `noindex` et exclut les téléchargements.
- Aucun lien ou asset local manquant, aucun `onclick` inline, scripts et
  manifestes valides.
- En-tête conditionnel `X-Robots-Tag: noindex` pour le sous-domaine OVH de
  préproduction.
- CSP publique et privée, répertoires non listables et lien vers les empreintes
  SHA-256 des téléchargements.

## Ancienne version OVH observée

- Certificat TLS ne correspondant pas à
  `relaist.cluster129.hosting.ovh.net` (`SEC_E_WRONG_PRINCIPAL`).
- Page en `index, follow`, sans en-tête `X-Robots-Tag`.
- Aucun en-tête CSP ou en-tête de sécurité observé sur l'accueil, `/admin/` et
  `/technicien/`.
- Canonique de production correcte, mais version structurée encore à `1.4.0`
  et anciens attributs `onclick` toujours présents.
- `robots.txt` autorise l'exploration et le sitemap pointe correctement vers le
  futur domaine de production.

Ces observations ne sont pas des régressions de la version locale : elles
disparaîtront uniquement après son déploiement.

## Mise en production

1. Déployer le contenu local `relaisdesk/` sur l'hébergement choisi.
2. Faire pointer `relaisdesk.fr` et `www.relaisdesk.fr`, installer leurs
   certificats et rediriger HTTP, `www` et `/index.html` en 301 vers la canonique.
3. Garder la préproduction en `noindex` et corriger son certificat si elle reste
   accessible.
4. Vérifier en ligne les statuts 200, CSP, `X-Robots-Tag`, `robots.txt`, sitemap,
   données structurées et Core Web Vitals.
5. Déclarer le domaine définitif dans Google Search Console et soumettre
   `https://relaisdesk.fr/sitemap.xml` seulement après la bascule DNS.

Le format AVIF du visuel social est léger mais mérite un test sur les réseaux
ciblés ; une image Open Graph PNG/JPEG de 1200×630 peut être ajoutée si leurs
aperçus ne prennent pas correctement l'AVIF en charge.
