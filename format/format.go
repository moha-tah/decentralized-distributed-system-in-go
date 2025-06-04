package format

import (
	"os"
	"log"
	"fmt"
)

// Codes pour le terminal
var rouge string = "\033[1;31m"
var orange string = "\033[1;33m"
var raz string = "\033[0;00m"
var green string = "\033[0;32m"

// Specific color for network layer
var networkColor string = "\033[1;34m" // Blue color for network messages

var pid = os.Getpid()
var stderr = log.New(os.Stderr, "", 0)


func Format_d(where string, who string, what string) string {
	return fmt.Sprintf("%s [+] [%12.12s %d] %-15.15s : %s\n",raz, who, pid, where, what)
}
func Format_w(where string, who string, what string) string {
	return fmt.Sprintf("%s [*] [%12.12s %d] %-15.15s : %s\n%s", orange, who, pid, where, what, raz)
}
func Format_e(where string, who string, what string) string {
    return fmt.Sprintf("%s [!] [%12.12s %d] %.15s : %s\n%s", rouge, who, pid, where, what, raz)
}
func Format_g(where string, who string, what string) string {
    return fmt.Sprintf("%s [✓] [%12.12s %d] %.15s : %s\n%s", green, who, pid, where, what, raz)
}

func Format_network(where string, who string, what string) string {
    return fmt.Sprintf("%s [󰒍] [%12.12s %d] %-15.15s : %s\n%s", networkColor, who, pid, where, what, raz)
}

func Display(message string) {
	stderr.Printf("%s\n", message)
}



func Display_d(where string, who string, what string) {
	stderr.Printf(Format_d(where, who, what))
}

func Display_e(where string, who string, what string) {
	stderr.Printf(Format_e(where, who, what))
}

func Display_g(where string, who string, what string) {
	stderr.Printf(Format_g(where, who, what))
}

func Display_w(where string, who string, what string) {
	stderr.Printf(Format_w(where, who, what))
}

func Display_network(where string, who string, what string) {
	stderr.Printf(Format_network(where, who, what))
}

