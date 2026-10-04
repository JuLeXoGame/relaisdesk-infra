# Recette Stripe Connect en mode test

Parcours complet : prestataire test → onboarding Stripe → activation →
prestation → Checkout test → webhook → statut `payé`. Aucun fond réel :
clés `sk_test`, carte `4242 4242 4242 4242`, base SQLite jetable dans `/tmp`.

Durée estimée : ~45 min (dont ~10 min d'actions navigateur).
Référence : [service_billing.go](../api/handlers/service_billing.go).

## Prérequis

- WSL Ubuntu avec ce dépôt (commandes `bash` ci-dessous, sauf mention).
- Go ≥ 1.26 dans WSL (une seule fois) :

```bash
GO_VERSION=$(curl -fsSL https://go.dev/VERSION?m=text | head -n1)
curl -fsSL -o /tmp/go.tgz "https://go.dev/dl/${GO_VERSION}.linux-amd64.tar.gz"
rm -rf "$HOME/go-tool" && mkdir -p "$HOME/go-tool" \
  && tar -C "$HOME/go-tool" -xzf /tmp/go.tgz
export PATH="$HOME/go-tool/go/bin:$PATH"
go version
```

- Stripe CLI sur Windows : `winget install stripe.stripe-cli`, puis
  `stripe login` (compte RelaisDesk).
- Une clé test Stripe : Dashboard en mode **Test** → Developers → API keys →
  `sk_test_...` (ne jamais utiliser une clé live ici).

## 0. Portes de compilation et tests

```bash
cd /home/julien/projet/keygen && go vet ./... && go test ./...
cd ../api && go build -o /tmp/recette-api . \
  && cd ../keygen && go build -o /tmp/recette-keygen .
```

`go test` doit être vert (dont `TestAddServerKey`) avant de continuer.

## 1. Fichier d'environnement de recette

Créer `/tmp/recette-api.env` (test uniquement, jamais en production) :

```bash
cat > /tmp/recette-api.env <<'EOF'
DEV_HTTP=true
API_BIND=127.0.0.1
API_PORT=8080
DB_PATH=/tmp/recette-relaisdesk.db
LOG_FILE=/tmp/recette-api.log
ADMIN_TOKEN=METTRE_48_CARACTERES_ALEATOIRES_ICI
NETWORK_AUTH_PRIVATE_KEY_FILE=/tmp/recette-network-auth-ed25519
STRIPE_SECRET_KEY=sk_test_METTRE_VOTRE_CLE_TEST_ICI
STRIPE_WEBHOOK_SECRET=whsec_recette_dummy
STRIPE_CONNECT_WEBHOOK_SECRET=whsec_METTRE_CELUI_DE_LA_CLI_ETAPE_3
SERVICE_PAYMENTS_ENABLED=true
PUBLIC_WEBSITE_URL=https://relaisdesk.fr
EOF
```

Générer le jeton admin :

```bash
python3 -c "import secrets; print(secrets.token_hex(24))"
# Coller le résultat à la place de METTRE_48_CARACTERES_ALEATOIRES_ICI.
```

## 2. Clé d'autorisation réseau locale

```bash
cd /home/julien/projet/api \
  && go run ./cmd/network-keygen -out /tmp/recette-network-auth-ed25519
```

(La clé publique affichée est inutile pour cette recette.)

## 3. Stripe CLI (PowerShell Windows, fenêtre gardée ouverte)

```powershell
stripe listen --forward-connect-to http://localhost:8080/api/v1/stripe/connect-webhook --events checkout.session.completed,checkout.session.async_payment_succeeded
```

Noter le secret `whsec_...` affiché et le coller dans
`/tmp/recette-api.env` (`STRIPE_CONNECT_WEBHOOK_SECRET`). Aucun endpoint
Dashboard n'est nécessaire en mode test : la CLI relaie vers le PC.

## 4. Base et fixtures (WSL)

```bash
rm -f /tmp/recette-relaisdesk.db*
/tmp/recette-keygen --db /tmp/recette-relaisdesk.db generate \
  --email recette@example.test --days 30 --max_connections 2
# Noter LICENSE ID et LICENSE KEY affichés.
/tmp/recette-keygen --db /tmp/recette-relaisdesk.db server-key \
  --add RECETTE_SERVER_KEY
set -a; source /tmp/recette-api.env; set +a
printf '%s' 'Recette-Test-2026' | /tmp/recette-api set-customer-password recette@example.test
```

## 5. Démarrer l'API (2e terminal WSL, gardé ouvert)

```bash
set -a; source /tmp/recette-api.env; set +a
/tmp/recette-api
# Attendu : "Starting HTTP server", aucune ligne FATAL.
curl -s http://localhost:8080/api/v1/health
```

## 6. Connexion propriétaire

```bash
OWNER_TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/customer/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"recette@example.test","password":"Recette-Test-2026"}' \
  | python3 -c "import json,sys; print(json.load(sys.stdin)['token'])")
[ -n "$OWNER_TOKEN" ] && echo "OWNER OK"
```

## 7. Conditions de prestations

```bash
TERMS_JSON=$(curl -s http://localhost:8080/api/v1/customer/service-billing \
  -H "Authorization: Bearer $OWNER_TOKEN")
echo "$TERMS_JSON" | python3 -m json.tool   # available doit être true
VERSION=$(echo "$TERMS_JSON" | python3 -c "import json,sys; print(json.load(sys.stdin)['terms']['version'])")
SHA=$(echo "$TERMS_JSON" | python3 -c "import json,sys; print(json.load(sys.stdin)['terms']['sha256'])")
curl -s -X POST http://localhost:8080/api/v1/customer/service-billing/terms/accept \
  -H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"accepted":true,"version":"'"$VERSION"'","sha256":"'"$SHA"'"}'
# Attendu : {"ok":true}
```

## 8. Onboarding Stripe (action navigateur nº 1)

```bash
ONBOARD_URL=$(curl -s -X POST http://localhost:8080/api/v1/customer/service-billing/onboarding \
  -H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' \
  -d '{}' | python3 -c "import json,sys; print(json.load(sys.stdin)['url'])")
echo "$ONBOARD_URL"
```

Ouvrir l'URL, compléter l'inscription avec des **données de test**
(bandeau jaune du mode test). Le retour aboutit sur le vrai site
`relaisdesk.fr` : c'est normal, revenir au terminal puis activer :

```bash
curl -s -X PUT http://localhost:8080/api/v1/customer/service-billing \
  -H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"enabled":true}'
# Attendu : {"ok":true} (409 = onboarding incomplète, recommencer le lien).
```

## 9. Tarif prépayé

```bash
RATE_ID=$(curl -s -X POST http://localhost:8080/api/v1/customer/service-billing/rates \
  -H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"label":"Dépannage test","mode":"prepaid","cents":150}' \
  | python3 -c "import json,sys; print(json.load(sys.stdin)['id'])")
echo "RATE=$RATE_ID"   # HTTP 201 attendu
```

## 10. Technicien, code viewer et prestation

```bash
TECH_TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/technician/login \
  -H 'Content-Type: application/json' \
  -d '{"license_id":"METTRE_LICENSE_ID","license_key":"METTRE_LICENSE_KEY"}' \
  | python3 -c "import json,sys; print(json.load(sys.stdin)['token'])")
[ -n "$TECH_TOKEN" ] && echo "TECH OK"
CODE=$(curl -s -X POST http://localhost:8080/api/v1/technician/viewer-codes/generate \
  -H "Authorization: Bearer $TECH_TOKEN" -H 'Content-Type: application/json' \
  -d '{"client_email":"client@example.test"}' \
  | python3 -c "import json,sys; print(json.load(sys.stdin)['code'])")
DEVICE_KEY=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
curl -s -X POST http://localhost:8080/api/v1/viewer/network-token \
  -H 'Content-Type: application/json' \
  -d '{"code":"'"$CODE"'","device_public_key":"'"$DEVICE_KEY"'"}'
curl -s -X POST http://localhost:8080/api/v1/viewer/announce \
  -H 'Content-Type: application/json' \
  -d '{"code":"'"$CODE"'","rustdesk_id":"123456789","device_public_key":"'"$DEVICE_KEY"'"}'
# Attendu : {"status":"success",...}
WORK_ID=$(curl -s -X POST http://localhost:8080/api/v1/technician/service-billing \
  -H "Authorization: Bearer $TECH_TOKEN" -H 'Content-Type: application/json' \
  -d '{"rate_id":"'"$RATE_ID"'","target_kind":"code","target_id":"'"$CODE"'","client_agreed":true}' \
  | python3 -c "import json,sys; print(json.load(sys.stdin)['id'])")
echo "WORK=$WORK_ID"   # HTTP 201 attendu, state=prepared, 1,50 €
```

## 11. Paiement test (action navigateur nº 2) et verdict

```bash
CHECKOUT_URL=$(curl -s -X POST http://localhost:8080/api/v1/customer/service-billing/work/$WORK_ID/checkout \
  -H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' -d '{}' \
  | python3 -c "import json,sys; print(json.load(sys.stdin)['checkout_url'])")
echo "$CHECKOUT_URL"
```

Ouvrir l'URL, payer avec `4242 4242 4242 4242` (date future, CVC
quelconque). La fenêtre `stripe listen` doit afficher
`checkout.session.completed` → `200`. Vérifier le statut :

```bash
curl -s http://localhost:8080/api/v1/technician/service-billing/$WORK_ID \
  -H "Authorization: Bearer $TECH_TOKEN" | python3 -m json.tool
# Attendu : "paid": true. RECETTE NOMINALE OK.
```

Cas négatifs (même session) :

- 2e prestation → checkout → bouton retour/annulation Stripe →
  `paid` reste `false`, aucun débit.
- Tarif à 40 centimes → `409` (minimum Stripe 0,50 €).
- Dans le Dashboard (mode test), le paiement est visible sur le compte
  connecté test, pas sur la plateforme.

## 12. Nettoyage

Arrêter l'API (`Ctrl+C`) et `stripe listen` (`Ctrl+C`), puis :

```bash
rm -f /tmp/recette-api.env /tmp/recette-relaisdesk.db* /tmp/recette-api.log \
  /tmp/recette-network-auth-ed25519 /tmp/recette-api /tmp/recette-keygen \
  /tmp/go.tgz
```

Les objets Stripe de test (compte, sessions) restent dans le bac à sable
sans conséquence ; suppression facultative depuis le Dashboard test.

## Dépannage

- La CLI n'atteint pas l'API (`Failed to connect`) : dans WSL le relais
  `localhost` vers Windows est normalement automatique ; sinon passer
  `API_BIND=0.0.0.0` (**test uniquement**) et redémarrer l'API.
- `stripe listen` affiche `400` : `STRIPE_CONNECT_WEBHOOK_SECRET` ne
  correspond pas au secret affiché par la CLI → corriger, redémarrer l'API.
- `PUT enabled` → `409` : onboarding incomplète. Régénérer un lien
  (`POST /onboarding` à volonté) et terminer l'inscription test.
- `429` sur les logins : limiteurs anti-abus (5/min) → attendre une minute.
- Ne jamais réutiliser ce fichier `.env` ni cette base en production :
  tout est jetable et en `sk_test`.
