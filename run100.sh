#!/usr/bin/env bash
# run100.sh — roda um exemplo 100 vezes e reporta estatísticas
#
# Uso: bash run100.sh [-q] <diretório-do-exemplo>
# Ex:  bash run100.sh Exemplos/flag_two
#      bash run100.sh -q Exemplos/flag_one_b

set -o pipefail

usage() {
    echo "Uso: run100.sh [-q|--quiet] <diretório-do-exemplo>"
    echo "  -q, --quiet   suprime saída individual, mostra só o resumo"
    exit 1
}

QUIET=0
DIR=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        -q|--quiet) QUIET=1; shift ;;
        -*) echo "flag desconhecida: $1"; usage ;;
        *)  [[ -z "$DIR" ]] && DIR="$1" || { echo "diretório já definido: $DIR (extra: $1)"; usage; }
            shift ;;
    esac
done

[[ -z "$DIR" ]] && usage

DIR=$(realpath "$DIR" 2>/dev/null || echo "$DIR")
[[ ! -d "$DIR" ]] && { echo "erro: '$DIR' não é um diretório"; exit 1; }
[[ ! -f "$DIR/go.mod" ]] && { echo "erro: '$DIR' não tem go.mod — não parece um exemplo Go"; exit 1; }

TOTAL=100
OK=0
FAIL=0
TIMEOUT=0
MIN_MS=999999
MAX_MS=0
TOTAL_MS=0
TIMEOUT_SEC=8

# Detecta se temos GNU date (ms) ou BSD/macOS
if date +%3N &>/dev/null; then
    now_ms() { echo $(( $(date +%s%3N) )); }
else
    now_ms() { echo $(( $(date +%s) * 1000 )); }
fi

echo "════════════════════════════════════════════════════════"
echo "  run100: $DIR × $TOTAL execuções"
echo "  timeout: ${TIMEOUT_SEC}s  |  quiet: $QUIET"
echo "════════════════════════════════════════════════════════"
echo

for ((i=1; i<=TOTAL; i++)); do
    START_TS=$(now_ms)

    TMPFILE=$(mktemp /tmp/run100_XXXXXX)

    # executa com timeout; go run captura stdout+stderr; exit code na última linha
    timeout "$TIMEOUT_SEC" bash -c "cd '$DIR'; go run . 2>&1; echo ___EXIT:\$?" > "$TMPFILE" 2>/dev/null
    TIMEOUT_CODE=$?

    END_TS=$(now_ms)
    ELAPSED=$((END_TS - START_TS))

    if [[ $TIMEOUT_CODE -eq 124 ]]; then
        ((TIMEOUT++))
        (( QUIET == 0 )) && printf "[%3d] ⏱  TIMEOUT\n" "$i"
        rm -f "$TMPFILE"
        continue
    fi

    # extrai exit code da última linha
    CODE=$(grep -oP '___EXIT:\K\d+' "$TMPFILE" 2>/dev/null)
    CODE=${CODE:-?}

    # saída sem a linha ___EXIT
    OUTPUT=$(grep -v '___EXIT:' "$TMPFILE" 2>/dev/null)
    rm -f "$TMPFILE"

    # atualiza min/max
    (( ELAPSED < MIN_MS )) && MIN_MS=$ELAPSED
    (( ELAPSED > MAX_MS )) && MAX_MS=$ELAPSED
    TOTAL_MS=$((TOTAL_MS + ELAPSED))

    # sucesso: exit 0 + contém FLAG
    IS_OK=0
    if [[ "$CODE" == "0" ]] && echo "$OUTPUT" | grep -q "FLAG"; then
        IS_OK=1
    fi

    if [[ $IS_OK -eq 1 ]]; then
        ((OK++))
        if (( QUIET == 0 )); then
            FLAG_LINE=$(echo "$OUTPUT" | grep "FLAG" | head -1 | tr -d '\n\r')
            printf "[%3d] ✓ OK  (%4dms)  %s\n" "$i" "$ELAPSED" "$FLAG_LINE"
        fi
    else
        ((FAIL++))
        if (( QUIET == 0 )); then
            LAST_FEW=$(echo "$OUTPUT" | tail -3 | tr '\n' '|' | head -c 120)
            printf "[%3d] ✗ FAIL (%4dms)  code=%s\n       %s\n" "$i" "$ELAPSED" "$CODE" "$LAST_FEW"
        fi
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
