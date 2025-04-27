#!/usr/bin/env bash
set -euo pipefail

# Usage check
if [ $# -lt 1 ]; then
  cat >&2 <<EOF
Usage: $0 NAME1:argA1,argA2… [NAME2:argB1,argB2…] …

Example:
  $0 \
    A:-node_type,sensor \
    B:-node_type,verifier,-node_name,verifier_1 \
    C:-node_type,user,-node_name,user_1
EOF
  exit 1
fi

# 1) Parse node definitions into arrays
declare -a NAMES=()        # e.g. (A B C)
declare -A OPTS            # OPTS[A]="-node_type,sensor"
for def in "$@"; do
  name="${def%%:*}"
  raw="${def#*:}"
  NAMES+=("$name")
  OPTS["$name"]="$raw"
done

# 2) Create all FIFOs
for name in "${NAMES[@]}"; do
  fifo="/tmp/in_$name"
  [[ -p $fifo ]] || mkfifo "$fifo"
done

# 3) Launch each node in the background
for name in "${NAMES[@]}"; do
  # split the comma-list back into an array of args
  IFS=',' read -r -a node_args <<< "${OPTS[$name]}"

  # build list of other nodes' FIFOs
  dests=()
  for other in "${NAMES[@]}"; do
    [[ $other != $name ]] && dests+=( "/tmp/in_$other" )
  done

  # run the program
  ./main "${node_args[@]}" \
    < "/tmp/in_$name" \
    | tee "${dests[@]}" &
done

# 4) Cleanup function
cleanup() {
  echo "⏹ Shutting down…"
  # kill all children of this script
  pkill -P $$ 2>/dev/null || true

  # remove FIFOs
  rm -f /tmp/in_* /tmp/out_*
  exit 0
}

# 5) Trap signals
trap cleanup INT QUIT TERM

echo "✅ Launched ${#NAMES[@]} nodes: ${NAMES[*]}"
echo "   (hit Ctrl+C to stop & clean up)"

# 6) Wait for all background processes
wait

