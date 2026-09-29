# Endpoint de statut public

La page de statut publique (Better Stack, `https://status.relaisdesk.fr/`,
liée depuis le pied de page du site) peut superviser l'API via l'endpoint
agrégé ci-dessous, en complément des sondes TCP directes.

## Contrat

`GET https://api.relaisdesk.fr/api/v1/public/status` (sans authentification,
60 requêtes/minute max, réponse cachée 30 s côté serveur).

```json
{
  "status": "operational",
  "checked_at": "2026-09-27T00:00:00Z",
  "services": [
    {"name": "api", "label": "API RelaisDesk", "status": "operational", "latency_ms": 0},
    {"name": "remote", "label": "Connexion à distance", "status": "operational", "latency_ms": 4},
    {"name": "notifications", "label": "Notifications e-mail", "status": "operational", "latency_ms": 12}
  ]
}
```

- `status` global : `operational` (tout OK), `degraded` (relais ou e-mail en
  panne), `outage` (API/base indisponible).
- `remote` couvre le rendez-vous (hbbs) ET le relais (hbbr) : `outage` si l'un
  des deux ne répond pas.
- La réponse ne contient ni hôtes, ni ports, ni versions (données publiques
  sans danger).

## Ajout dans Better Stack

1. Uptime → Monitors → Create monitor, type **HTTP(S)**.
2. URL : `https://api.relaisdesk.fr/api/v1/public/status`.
3. Vérification du contenu (Reminder/keyword check) : le corps doit contenir
   `"status":"operational"`.
4. Intervalle : 1 minute (l'endpoint est caché 30 s, sans risque de surcharge).
5. Attacher le moniteur à la page de statut publique existante.

Code : `api/handlers/public_status.go`, route dans `api/main.go`,
tests `api/handlers/public_status_test.go`.
