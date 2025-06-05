#!/usr/bin/env bash
set -euo pipefail

# Usage check
if [ $# -lt 1 ]; then
  cat >&2 <<EOF
Usage: $0 NAME1:argA1,argA2… [NAME2:argB1,argB2…] 

Example:
  $0 \
    A:-node_type,sensor \
    B:-node_type,verifier \
    C:-node_type,user
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

# 3) Launch each node in the background (with computed node_id)
for idx in "${!NAMES[@]}"; do
  name="${NAMES[$idx]}"
  # split the comma-list back into an array of args
  IFS=',' read -r -a node_args <<< "${OPTS[$name]}"

  # In a bidirectional ring: each node sends to BOTH neighbors
  # Next node (clockwise)
  next_idx=$(( (idx + 1) % ${#NAMES[@]} ))
  next_name="${NAMES[$next_idx]}"
  next_fifo="/tmp/in_$next_name"
  
  # Previous node (counter-clockwise)
  prev_idx=$(( (idx - 1 + ${#NAMES[@]}) % ${#NAMES[@]} ))
  prev_name="${NAMES[$prev_idx]}"
  prev_fifo="/tmp/in_$prev_name"

  # build main
  go build -o main main.go

  # run the program with --node_id, output goes to BOTH neighbors AND shows on terminal
  ./main --node_id "$idx" "${node_args[@]}" \
    < "/tmp/in_$name" \
    | tee "$next_fifo" "$prev_fifo" &
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

echo -e "\n\n✅ Launched ${#NAMES[@]} nodes in bidirectional ring topology: ${NAMES[*]}"
echo -e "   Ring connections: "
for idx in "${!NAMES[@]}"; do
  name="${NAMES[$idx]}"
  next_idx=$(( (idx + 1) % ${#NAMES[@]} ))
  prev_idx=$(( (idx - 1 + ${#NAMES[@]}) % ${#NAMES[@]} ))
  next_name="${NAMES[$next_idx]}"
  prev_name="${NAMES[$prev_idx]}"
  echo -e "   $name ↔ $next_name, $prev_name"
done
echo -e "   (hit Ctrl+C to stop & clean up)\n\n"

# 6) Wait for all background processes
wait
