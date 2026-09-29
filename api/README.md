# RelaisDesk License API

API HTTP de gestion et validation des licences RelaisDesk.

Les commandes de consommateurs sont désactivées par défaut. Ne définir
`B2C_SALES_ENABLED=true` qu'après avoir renseigné `LEGAL_PHONE`,
`CONSUMER_MEDIATOR_NAME` et `CONSUMER_MEDIATOR_URL`, puis publié exactement les
mêmes coordonnées dans les mentions légales et les CGV.

## Configuration

```bash
API_BIND=127.0.0.1
API_PORT=8443
DB_PATH=/data/relaisdesk/licences.db
ADMIN_TOKEN=CHANGE_ME_WITH_A_VERY_LONG_RANDOM_TOKEN
SERVER_IP=api.relaisdesk.fr
RUSTDESK_RENDEZVOUS_PORT=21116
RUSTDESK_RELAY_PORT=21117
TLS_CERT=/opt/relaisdesk/certs/fullchain.pem
TLS_KEY=/opt/relaisdesk/certs/privkey.pem
LOG_FILE=/var/log/relaisdesk/api.log
CLIENT_SOURCE_URL=https://github.com/JuLeXoGame/rustdesk/tree/c551990c69c95430442569d15b277fb61c6fe0e0
SERVER_SOURCE_URL=https://github.com/JuLeXoGame/rustdesk-server/tree/57318afae9c19935c75e47700a4d69275b71cce8
LEGAL_PHONE=06 62 85 59 30
B2C_SALES_ENABLED=false
CONSUMER_MEDIATOR_NAME=
CONSUMER_MEDIATOR_URL=
PUBLIC_WEBSITE_URL=https://relaisdesk.fr
EMAIL_DOMAIN=relaisdesk.fr
SMTP_DKIM_SELECTOR=<selecteur-dkim-du-prestataire>
EMAIL_REQUIRE_DNS_AUTH=true
```

En production, les deux URL de code source correspondant et le téléphone
professionnel sont obligatoires. L'API refuse de démarrer et les
téléchargements restent fermés tant que les forks complets ne sont pas publiés
en HTTPS conformément à l'AGPLv3.
L'écoute de production est limitée à une adresse de bouclage ; Nginx Oracle
publie ensuite l'API sur `443`.

## Endpoints publics

- `POST /api/v1/activate`
- `POST /api/v1/validate`
- `POST /api/v1/heartbeat`
- `GET /api/v1/health`
- `POST /api/v1/public/order`
- `POST /api/v1/public/withdrawal`
- `POST /api/v1/customer/login/request`
- `POST /api/v1/customer/login/verify`
- `POST /api/v1/stripe/webhook` (Stripe uniquement, signature obligatoire)
- `POST /api/v1/stripe/connect-webhook` (Stripe Connect uniquement, signature obligatoire)

En production, enregistrer exactement
`https://api.relaisdesk.fr/api/v1/stripe/webhook` dans Stripe Workbench pour
l'événement `checkout.session.completed`, puis placer le secret `whsec_...` de
cette destination dans `STRIPE_WEBHOOK_SECRET`. La route
`/api/v1/webhooks/stripe` n'existe pas. Le retour Checkout vers la page de
succès ne remplace pas la livraison du webhook.

Pour les prestations (Stripe Connect), enregistrer exactement
`https://api.relaisdesk.fr/api/v1/stripe/connect-webhook` comme destination
écoutant les événements **des comptes connectés** (pas ceux du compte
plateforme), pour `checkout.session.completed` et
`checkout.session.async_payment_succeeded`, puis placer son secret `whsec_...`
dans `STRIPE_CONNECT_WEBHOOK_SECRET` avec `SERVICE_PAYMENTS_ENABLED=true`.
Recette en mode test : `docs/RECETTE_CONNECT_TEST.md`.

## Endpoints espace client

Tous les endpoints ci-dessous utilisent la session opaque obtenue par le lien
e-mail, jamais la clé de licence technicien :

- `GET /api/v1/customer/dashboard`
- `PUT /api/v1/customer/preferences`
- `POST /api/v1/customer/licenses/{license_id}/renew`
- `GET /api/v1/customer/invoices/{invoice_number}/download`
- `GET|POST /api/v1/customer/interventions`
- `GET /api/v1/customer/interventions/export`

Les renouvellements sont des paiements express : Stripe ou virement. La
licence existante est prolongée automatiquement après confirmation, sans débit
récurrent tacite. Le processus horaire de l'API envoie et déduplique les
relances d'échéance. En préproduction, `PUBLIC_WEBSITE_URL` doit désigner le
domaine OVH réellement accessible afin que les liens e-mail ouvrent
`/client/`; en production il devra être remplacé par `https://relaisdesk.fr`.

Le webhook Stripe et tous les courriels commerciaux utilisent une file SQLite
persistante avec reprise exponentielle. Il n'y a aucun courtier ni agent de
supervision supplémentaire. Les travaux terminés ou définitivement échoués
sont supprimés après trente jours.

Les données commerciales sont isolées par `customer_id`. L'adresse e-mail
authentifie un membre de `customer_users`, mais n'est plus l'identifiant du
compte client.

## Endpoints admin

Tous les endpoints admin demandent :

```http
Authorization: Bearer <ADMIN_TOKEN>
```

- `GET /api/v1/admin/licences`
- `POST /api/v1/admin/licences`
- `POST /api/v1/admin/licences/{license_id}/revoke`
- `GET /api/v1/admin/stats`
- `GET /api/v1/admin/withdrawals`

## Build

```bash
go build -o api .
go test -v ./...
```

Deux outils d'exploitation sont également construits par
`scripts/deploy_api.sh` :

```bash
# Contrôle ponctuel SPF/DKIM/DMARC
relaisdesk-emailcheck -domain relaisdesk.fr -selector <selecteur> -strict=true

# Sauvegarde SQLite cohérente, chiffrée et vérifiée
relaisdesk-backup create
```

Voir `docs/ADMIN.md` avant toute restauration.
