#!/usr/bin/env bash
# run100.sh — executa um binário 100 vezes e reporta estatísticas
#
# Uso: bash run100.sh [-q] <binário>
# Ex:  bash run100.sh ./flag_two
#      bash run100.sh -q ./meu_programa

set -o pipefail

usage() {
    echo "Uso: run100.sh [-q|--quiet] <binário>"
    echo "  -q, --quiet   suprime saída individual, mostra só o resumo"
    exit 1
}

QUIET=0
BIN=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        -q|--quiet) QUIET=1; shift ;;
        -*) echo "flag desconhecida: $1"; usage ;;
        *)  [[ -z "$BIN" ]] && BIN="$1" || { echo "binário já definido: $BIN (extra: $1)"; usage; }
            shift ;;
    esac
done

[[ -z "$BIN" ]] && usage

BIN=$(realpath "$BIN" 2>/dev/null || echo "$BIN")
[[ ! -f "$BIN" ]] && { echo "erro: '$BIN' não encontrado"; exit 1; }
[[ ! -x "$BIN" ]] && { echo "erro: '$BIN' não é executável"; exit 1; }

TOTAL=100
OK=0
FAIL=0
TIMEOUT=0
MIN_MS=999999
MAX_MS=0
TOTAL_MS=0
TIMEOUT_SEC=8

if date +%3N &>/dev/null; then
    now_ms() { echo $(( $(date +%s%3N) )); }
else
    now_ms() { echo $(( $(date +%s) * 1000 )); }
fi

echo "════════════════════════════════════════════════════════"
echo "  run100: $BIN × $TOTAL execuções"
echo "  timeout: ${TIMEOUT_SEC}s  |  quiet: $QUIET"
echo "════════════════════════════════════════════════════════"
echo

for ((i=1; i<=TOTAL; i++)); do
    START_TS=$(now_ms)

    timeout "$TIMEOUT_SEC" "$BIN" > /dev/null 2>&1
    EXIT_CODE=$?

    END_TS=$(now_ms)
    ELAPSED=$((END_TS - START_TS))

    if [[ $EXIT_CODE -eq 124 ]]; then
        ((TIMEOUT++))
        (( QUIET == 0 )) && printf "[%3d] ⏱  TIMEOUT\n" "$i"
        continue
    fi

    (( ELAPSED < MIN_MS )) && MIN_MS=$ELAPSED
    (( ELAPSED > MAX_MS )) && MAX_MS=$ELAPSED
    TOTAL_MS=$((TOTAL_MS + ELAPSED))

    if [[ $EXIT_CODE -eq 0 ]]; then
        ((OK++))
        (( QUIET == 0 )) && printf "[%3d] ✓ OK  (%4dms)\n" "$i" "$ELAPSED"
    else
        ((FAIL++))
        (( QUIET == 0 )) && printf "[%3d] ✗ FAIL (%4dms)  exit=%d\n" "$i" "$ELAPSED" "$EXIT_CODE"
    fi
done

# --- resumo ---
ATTEMPTED=$((OK + FAIL))
if [[ $TOTAL -gt 0 ]]; then
    SUCCESS_RATE=$(awk "BEGIN {printf \"%.0f\", ($OK * 100) / $TOTAL}")
else
    SUCCESS_RATE=0
fi
if [[ $ATTEMPTED -gt 0 ]]; then
    AVG_MS=$((TOTAL_MS / ATTEMPTED))
else
    AVG_MS=0
fi

echo
echo "════════════════════════════════════════════════════════"
echo "  RESULTADO"
echo "════════════════════════════════════════════════════════"
echo
printf "  Total:      %4d\n" "$TOTAL"
printf "  Sucesso:    %4d  (%s%%)\n" "$OK" "$SUCCESS_RATE"
printf "  Falha:      %4d\n" "$FAIL"
printf "  Timeout:    %4d\n" "$TIMEOUT"
echo
printf "  Tempo mínimo: %4dms\n" "$MIN_MS"
printf "  Tempo máximo: %4dms\n" "$MAX_MS"
printf "  Tempo médio:  %4dms\n" "$AVG_MS"
echo "════════════════════════════════════════════════════════"

[[ $OK -eq $TOTAL ]] && exit 0 || exit 1
