#!/bin/bash
# launcher.sh: Launches multiple instances of ./main with argument separation and termination (Ctrl-C) handling.

# Array to store Process IDs (PIDs) of all launched 'main' instances.
declare -a PIDS=()

# --- Function to clean up all child processes on script exit or Ctrl-C ---
cleanup() {
    echo -e "\n--- Ctrl-C detected! Stopping all 'main' instances... ---"
    # Iterate through the stored PIDs
    for pid in "${PIDS[@]}"; do
        # Check if the process with this PID is still running
        if kill -0 "$pid" 2>/dev/null; then
            # If it's running, send a SIGTERM signal to gracefully terminate it.
            # If it doesn't terminate, a subsequent SIGKILL might be needed (not implemented here for simplicity).
            kill "$pid"
            echo "Sent termination signal to process $pid"
        else
            echo "Process $pid already terminated or not found."
        fi
    done
    echo "--- All 'main' instances stopped. Exiting. ---"
    exit 0 # Exit the launcher script
}

# --- Set up the trap for SIGINT (Ctrl-C) ---
# When SIGINT is received, the 'cleanup' function will be executed.
trap cleanup SIGINT

# --- Argument Parsing and Launching 'main' instances ---

# Join all arguments passed to this script into a single string.
# This allows to easily split them by the semicolon later.
all_input_args="$*"

# Split the combined arguments string by the '-' delimiter.
# 'IFS=-' temporarily sets the Internal Field Separator to ';' for this command.
# 'read -ra' reads into an array, treating each segment as an element.
IFS='-' read -ra COMMAND_SEGMENTS <<< "$all_input_args"

# Loop through each segment (which represents arguments for one 'main' instance)
for cmd_args_segment in "${COMMAND_SEGMENTS[@]}"; do
    # Trim leading/trailing whitespace from the segment using xargs.
    # This ensures clean arguments even if there's extra space around ';'.
    trimmed_args=$(echo "$cmd_args_segment" | xargs)

    # Only attempt to launch if the trimmed argument segment is not empty.
    if [ -n "$trimmed_args" ]; then
        echo "Launching new instance: ./main $trimmed_args"
        # Launch the 'main' script in the background.
        # The '&' symbol puts the command into the background.
        ./main $trimmed_args &
        # Store the PID of the last background process ($! contains the PID).
        PIDS+=($!)
    else 
	echo "[!] An instance could not be started: no parameters."
    fi
done

# --- Keep the launcher script running ---
echo ""
echo -e "✅ All 'main' instances launched. Press Ctrl-C at any time to stop them."
echo -e "------------------------------------------------------------------------\n\n"

# An infinite loop with a sleep command keeps the parent script alive.
# Needed for the 'trap' command to be able to catch Ctrl-C and clean up children.
while true; do
    sleep 1 # Sleep for 1 second to avoid busy-waiting
done

