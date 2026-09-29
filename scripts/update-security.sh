#!/usr/bin/env bash
# RelaisDesk - Maintenance et audit de securite des dependances
# (portage de update-security.ps1 pour Linux/WSL).
#
# Recherche un Python 3.11+, puis execute scripts/security_maintenance.py
# dans un espace isole (update/<run-id>). Aucune ecriture directe sur le
# projet sans confirmation : --apply-fixes et --rollback demandent une
# validation interactive, sauf --force ou --dry-run.
#
# Exemples :
#   bash scripts/update-security.sh --audit-only --show-report
#   bash scripts/update-security.sh --run-build-tests --show-report
#   bash scripts/update-security.sh --apply-fixes --show-report
#   bash scripts/update-security.sh --apply-fixes --dry-run --show-report
#   bash scripts/update-security.sh --rollback --run-id '20260919-120000-1234abcd' --show-report
set -euo pipefail

AUDIT_ONLY=0; PREPARE_ONLY=0; RUN_BUILD_TESTS=0; INSTALL_AUDIT_TOOLS=0
APPLY_FIXES=0; DRY_RUN=0; ROLLBACK=0; FORCE=0; SHOW_REPORT=0; OPEN_REPORT=0
TIMEOUT=900; PYTHON_PATH=""; RUN_ID=""; RUN_ID_GIVEN=0

while [ $# -gt 0 ]; do
    case "$1" in
        --audit-only) AUDIT_ONLY=1; shift ;;
        --prepare-only) PREPARE_ONLY=1; shift ;;
        --run-build-tests) RUN_BUILD_TESTS=1; shift ;;
        --install-audit-tools) INSTALL_AUDIT_TOOLS=1; shift ;;
        --apply-fixes) APPLY_FIXES=1; shift ;;
        --dry-run) DRY_RUN=1; shift ;;
        --rollback) ROLLBACK=1; shift ;;
        --force) FORCE=1; shift ;;
        --show-report) SHOW_REPORT=1; shift ;;
        --open-report) OPEN_REPORT=1; shift ;;
        --command-timeout-seconds) TIMEOUT="$2"; shift 2 ;;
        --python-path) PYTHON_PATH="$2"; shift 2 ;;
        --run-id) RUN_ID="$2"; RUN_ID_GIVEN=1; shift 2 ;;
        -h|--help) sed -n '2,16p' "$0"; exit 0 ;;
        *) echo "Option inconnue : $1" >&2; exit 1 ;;
    esac
done

if ! printf '%s' "$TIMEOUT" | grep -Eq '^[0-9]+$' || [ "$TIMEOUT" -lt 30 ] || [ "$TIMEOUT" -gt 7200 ]; then
    echo "Delai invalide (30 a 7200 secondes)." >&2; exit 1
fi
if [ "$AUDIT_ONLY" -eq 1 ] && [ "$PREPARE_ONLY" -eq 1 ]; then echo "Choisir --audit-only OU --prepare-only." >&2; exit 1; fi
if [ "$ROLLBACK" -eq 1 ]; then
    if [ "$RUN_ID_GIVEN" -eq 0 ]; then echo "--rollback exige --run-id : identifiant du lot applique a annuler." >&2; exit 1; fi
    if [ "$APPLY_FIXES" -eq 1 ] || [ "$AUDIT_ONLY" -eq 1 ] || [ "$PREPARE_ONLY" -eq 1 ] || [ "$RUN_BUILD_TESTS" -eq 1 ] || [ "$INSTALL_AUDIT_TOOLS" -eq 1 ]; then
        echo "--rollback est exclusif : aucun audit ni --apply-fixes." >&2; exit 1
    fi
else
    if [ "$DRY_RUN" -eq 1 ]; then APPLY_FIXES=1; fi
    if [ "$APPLY_FIXES" -eq 1 ] && { [ "$AUDIT_ONLY" -eq 1 ] || [ "$PREPARE_ONLY" -eq 1 ]; }; then
        echo "--apply-fixes exige une maintenance complete, pas --audit-only/--prepare-only." >&2; exit 1
    fi
    if [ "$APPLY_FIXES" -eq 1 ]; then RUN_BUILD_TESTS=1; fi
    if [ -z "$RUN_ID" ]; then RUN_ID="$(date +'%Y%m%d-%H%M%S-')$(tr -d '-' < /proc/sys/kernel/random/uuid)"; fi
fi
if ! printf '%s' "$RUN_ID" | grep -Eq '^[0-9]{8}-[0-9]{6}-[a-f0-9]{8,32}$'; then echo "RunId invalide." >&2; exit 1; fi

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_DIR="$PROJECT_ROOT/update/$RUN_ID"
if [ "$ROLLBACK" -eq 1 ]; then
    if [ ! -d "$RUN_DIR" ]; then echo "Lot '$RUN_ID' introuvable dans update/ : rien a annuler." >&2; exit 1; fi
elif [ -e "$RUN_DIR" ]; then
    echo "Identifiant deja utilise ; aucun lot ne sera ecrase." >&2; exit 1
fi

confirm_project_write() {
    if [ "$FORCE" -eq 1 ]; then return 0; fi
    if [ ! -t 0 ]; then echo "Session non interactive : relancer avec --force pour confirmer explicitement." >&2; exit 1; fi
    local answer=""
    read -r -p "$1 [o/N] " answer || true
    printf '%s' "$answer" | grep -Eq -i '^(o|oui|y|yes)$'
}

# Detection Python 3.11+ (ordre adapte a Linux ; $VIRTUAL_ENV/bin/python d'abord).
CANDIDATES=""
if [ -n "$PYTHON_PATH" ]; then
    if [ ! -f "$PYTHON_PATH" ] && ! command -v "$PYTHON_PATH" >/dev/null 2>&1; then
        echo "Chemin Python invalide : '$PYTHON_PATH' est introuvable (ni fichier ni commande)." >&2; exit 1
    fi
    CANDIDATES="$PYTHON_PATH"
else
    if [ -n "${VIRTUAL_ENV:-}" ] && [ -f "$VIRTUAL_ENV/bin/python" ]; then CANDIDATES="$CANDIDATES $VIRTUAL_ENV/bin/python"; fi
    for name in python3 python; do
        found="$(command -v "$name" 2>/dev/null || true)"
        if [ -n "$found" ]; then CANDIDATES="$CANDIDATES $found"; fi
    done
    for dir in "$HOME/.local/bin" "/usr/local/bin"; do
        for py in "$dir"/python3*; do
            if [ -f "$py" ] && [ -x "$py" ]; then CANDIDATES="$CANDIDATES $py"; fi
        done
    done
fi
SELECTED=""
for cand in $CANDIDATES; do
    if "$cand" -c 'import sys; raise SystemExit(0 if sys.version_info >= (3, 11) else 1)' 2>/dev/null; then
        SELECTED="$cand"; break
    fi
done
if [ -z "$SELECTED" ]; then
    echo "Python 3.11 ou plus recent requis (ex : paquet python3.11+, rustup equivalent uv, ou --python-path)." >&2; exit 1
fi

ARGS=(-B "$PROJECT_ROOT/scripts/security_maintenance.py" --project-root "$PROJECT_ROOT" --timeout "$TIMEOUT" --run-id "$RUN_ID")
if [ "$AUDIT_ONLY" -eq 1 ]; then ARGS+=(--audit-only); fi
if [ "$PREPARE_ONLY" -eq 1 ]; then ARGS+=(--prepare-only); fi
if [ "$RUN_BUILD_TESTS" -eq 1 ]; then ARGS+=(--run-build-tests); fi
if [ "$INSTALL_AUDIT_TOOLS" -eq 1 ]; then ARGS+=(--install-audit-tools); fi

EXIT_CODE=0
if [ "$ROLLBACK" -eq 0 ]; then
    "$SELECTED" "${ARGS[@]}" || EXIT_CODE=$?
fi
if { [ "$APPLY_FIXES" -eq 1 ] || [ "$ROLLBACK" -eq 1 ]; } && [ "$EXIT_CODE" -eq 0 ]; then
    APPLY_ARGS=(-B "$PROJECT_ROOT/scripts/maintenance_apply.py" --project-root "$PROJECT_ROOT" --run-id "$RUN_ID")
    if [ "$ROLLBACK" -eq 1 ]; then APPLY_ARGS+=(--rollback); QUESTION="Annuler le lot applique sur le projet"; else QUESTION="Appliquer les correctifs au projet"; fi
    if [ "$DRY_RUN" -eq 1 ]; then APPLY_ARGS+=(--dry-run); fi
    if [ "$DRY_RUN" -eq 1 ] || confirm_project_write "$QUESTION"; then
        "$SELECTED" "${APPLY_ARGS[@]}" || EXIT_CODE=$?
    else
        echo "Abandon sur confirmation : aucune modification du projet."
    fi
fi

if [ "$SHOW_REPORT" -eq 1 ] || [ "$OPEN_REPORT" -eq 1 ]; then
    REPORT="$RUN_DIR/rapport.md"
    if [ -f "$REPORT" ]; then
        if [ "$SHOW_REPORT" -eq 1 ]; then
            echo ""
            echo "--- SYNTHESE DU RAPPORT ($REPORT) ---"
            SYNTHESIS=$(awk '/^## Signalements restants/{exit} {print}' "$REPORT")
            if [ -z "$SYNTHESIS" ]; then SYNTHESIS=$(head -n 35 "$REPORT"); fi
            printf '%s\n' "$SYNTHESIS"
            grep -E '^\*\*NE PAS appliquer' "$REPORT" || true
            echo "Detail complet : $REPORT"
        fi
        if [ "$OPEN_REPORT" -eq 1 ]; then
            command -v xdg-open >/dev/null || { echo "xdg-open introuvable pour --open-report." >&2; exit 1; }
            xdg-open "$REPORT" >/dev/null 2>&1 &
        fi
    fi
fi

exit "$EXIT_CODE"
