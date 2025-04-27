#!/usr/bin/env bash
set -euo pipefail

# ----------------------------------------
# Usage
# ----------------------------------------
if [ $# -lt 1 ]; then
  cat >&2 <<EOF
Usage: $0 NAME1:argA1,argA2… [NAME2:argB1,argB2…] …

Spins up a unidirectional ring.  Each node N reads from /tmp/in_N,
writes to /tmp/out_N, and then we cat out_N into the next node’s in_*.

Example:
  $0 \
    A:-node_type,sensor \
    B:-node_type,verifier,-node_name,verifier_1 \
    C:-node_type,user,-node_name,user_1
EOF
  exit 1
fi

# ----------------------------------------
# 1) Parse node definitions
# ----------------------------------------
declare -a NAMES=()         # ordered list of node names
declare -A OPTS             # OPTS[A]="-node_type,sensor"

for def in "$@"; do
  name="${def%%:*}"
  raw="${def#*:}"
  NAMES+=("$name")
  OPTS["$name"]="$raw"
done
COUNT=${#NAMES[@]}

# ----------------------------------------
# 2) Create all FIFOs
# ----------------------------------------
for name in "${NAMES[@]}"; do
  mkfifo -m 600 "/tmp/in_$name"   || true
  mkfifo -m 600 "/tmp/out_$name"  || true
done

# ----------------------------------------
# 3) Launch each node:  prog < in -> out
# ----------------------------------------
for name in "${NAMES[@]}"; do
  # split the comma-list into an argv array
  IFS=',' read -r -a node_args <<< "${OPTS[$name]}"

  ./main "${node_args[@]}" \
    < "/tmp/in_$name" \
    > "/tmp/out_$name" &
done

# ----------------------------------------
# 4) Chain each out -> next in with cat
# ----------------------------------------
for i in "${!NAMES[@]}"; do
  src="${NAMES[$i]}"
  # next index wraps around
  next="${NAMES[$(( (i+1) % COUNT ))]}"
  cat "/tmp/out_$src" > "/tmp/in_$next" &
done

# ----------------------------------------
# 5) Cleanup function
# ----------------------------------------
cleanup() {
  echo "⏹ Shutting down…"

  # kill all children of this script
  pkill -P $$ 2>/dev/null || true

  # remove FIFOs
  rm -f /tmp/in_* /tmp/out_*

  exit 0
}

# ----------------------------------------
# 6) Trap signals & wait
# ----------------------------------------
trap cleanup INT QUIT TERM

echo -e "\n\n✅ Launched $COUNT-node unidirectional ring: ${NAMES[*]}"
echo -e "   (Ctrl+C to stop and clean up)\n\n"

wait

