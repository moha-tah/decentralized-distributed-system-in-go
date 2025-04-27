package node

import (
	"fmt"
	"bufio" // Use bufio to read full line, as fmt.Scanln split at new line AND spaces
	"os"
	"distributed_system/format"
)

func Verifier(verifier_name *string) {

	var rcvmsg string 
	reader := bufio.NewReader(os.Stdin)
	
	for {
		rcvmsg, _ = reader.ReadString('\n') // read until newline
		rcvmsg = rcvmsg[:len(rcvmsg)-1] // remove the trailing newline character
		format.Display(format.Format_d(
			"main", 
			*verifier_name,
			*verifier_name + " received <" + rcvmsg + ">"))

		fmt.Println(rcvmsg)
	}

}
