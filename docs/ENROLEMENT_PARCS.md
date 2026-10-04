# Enrôlement de masse (grands parcs)

Deux mécanismes coexistent :

| Cas | Mécanisme | Durée de vie |
|---|---|---|
| Un poste précis, assisté | Code `PERM-…` unique par poste | 15 minutes |
| Vague de déploiement (GPO/Intune/SCCM) | **Token de parc `PARK-…` multi-usage** | 30 jours par défaut (max 365) |

## 1. Créer un token de parc

```bash
# Session technicien au préalable (Bearer $TOKEN), licence $LIC :
curl -s -X POST https://api.relaisdesk.fr/api/v1/technician/device-park-tokens \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"license_id":"'"$LIC"'","label":"Vague janvier","max_uses":500,"ttl_days":30}'
# → {"token":"PARK-… (à copier, affiché une seule fois)","park_token":{…}}
```

Options : `folder_id` (range les postes dans un dossier), `label`.
Lister : `GET …/device-park-tokens`. Révoquer : `PUT …/device-park-tokens/{id}/revoke`
(les postes déjà enrôlés continuent de fonctionner).
Mêmes endpoints côté client : `/api/v1/customer/device-park-tokens`.

Chaque enrôlement consomme un slot du quota de la licence et décrémente
`max_uses` atomiquement (les vagues concurrentes ne dépassent pas le cap).
L'alias par défaut est le hostname (renommable ensuite).

## 2. Déployer

```bat
RelaisDesk_Setup.exe /S /ENROLLCODE=PARK-XXXX-… /PASSWORD=mot-de-passe-postes
```

- `/S` : silencieux (requis pour GPO/Intune/SCCM).
- `/ENROLLCODE=` : token de parc (ou code `PERM-…` pour un poste unique).
- `/PASSWORD=` : mot de passe permanent RustDesk (optionnel mais requis pour
  l'accès sans surveillance ; transite par variable d'environnement, jamais
  en argv, nettoyée après usage).
- En cas d'échec d'enrôlement, l'installeur sort en code 1 pour que l'outil
  de déploiement rejoue (l'enrôlement est réessayable sans `--unenroll`
  tant qu'il n'a pas abouti).

Sous Linux (script) : installer le `.deb` puis
`sudo relaisdesk-viewer --enroll PARK-…` (mot de passe via
`RELAISDESK_ENROLL_PASSWORD` ou `@fichier`).

## 3. Limites dimensionnées pour les parcs

- Enrôlement : 120/min par IP de sortie (codes ≥ 118 bits + TTL → devinette
  infaisable).
- Heartbeat (45 s) : 5/min par poste authentifié + garde-fou 6000/min par IP :
  un parc entier derrière un NAT n'est plus bridé.
- Mises à jour : échelonnement serveur (`stagger_seconds`) anti-troupeau.

## 4. Fin de vie d'un poste

Désinstaller appelle `--unenroll` (le slot quota est libéré). Pour retirer un
poste volé/perdu : supprimer le device dans la console (révocation immédiate).
