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

Méthode recommandée (aucun secret sur les lignes de commande, sinon visibles
via le Gestionnaire des tâches) :

```powershell
$env:RELAISDESK_ENROLL_CODE='PARK-XXXX-…'; $env:RELAISDESK_ENROLL_PASSWORD='mot-de-passe-postes'; .\RelaisDesk_Setup.exe /S
```

Repli :

```bat
RelaisDesk_Setup.exe /S /ENROLLCODE=PARK-XXXX-… /PASSWORD=mot-de-passe-postes
```

- `/S` : silencieux (requis pour GPO/Intune/SCCM).
- Code : token de parc (ou code `PERM-…` pour un poste unique), via
  `RELAISDESK_ENROLL_CODE` ou `/ENROLLCODE=`.
- Mot de passe permanent RustDesk : requis pour l'accès sans surveillance
  (sans lui, le mode silencieux échoue au lieu d'attendre) ; via
  `RELAISDESK_ENROLL_PASSWORD` ou `/PASSWORD=`. Transmis au viewer par
  l'environnement et nettoyé après usage.
- En cas d'échec d'enrôlement, l'installeur sort en code 1 pour que l'outil
  de déploiement rejoue. Le rejouement est idempotent : même poste + même
  clé = même fiche (aucun doublon, aucune consommation supplémentaire) ;
  une réinstallation (nouvelle clé) crée un nouveau poste.

Sous Linux (script, en root) : installer le `.deb` puis
`relaisdesk-viewer --enroll --silent` avec le code via
`RELAISDESK_ENROLL_CODE` (mot de passe via `RELAISDESK_ENROLL_PASSWORD` ou
`@fichier`). Via sudo, préserver l'environnement (`sudo -E …`) car sudo
filtre les variables par défaut. En interactif,
`sudo relaisdesk-viewer --enroll` demande le code.

## 3. Limites dimensionnées pour les parcs

- Enrôlement : 120/min par IP de sortie (codes ≥ 118 bits + TTL → devinette
  infaisable).
- Heartbeat (45 s) : 5/min par poste authentifié + garde-fou 6000/min par IP :
  un parc entier derrière un NAT n'est plus bridé.
- Mises à jour : échelonnement serveur (`stagger_seconds`) anti-troupeau.

## 4. Fin de vie d'un poste

Désinstaller appelle `--unenroll` (le slot quota est libéré). Pour retirer un
poste volé/perdu : supprimer le device dans la console (révocation immédiate).
