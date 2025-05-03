package format

import (
	"fmt"
	"log"
	"strings"
)

func Findval(msg string, key string, p_nom string) string {
	if len(msg) < 4 {
		stderr.Print(Format_w("findval", p_nom, "message trop court : "+msg))
		return ""
	}
	sep := msg[0:1]
	tab_allkeyvals := strings.Split(msg[1:], sep)
	for _, keyval := range tab_allkeyvals {
		equ := keyval[0:1]
		tabkeyval := strings.Split(keyval[1:], equ)
		if tabkeyval[0] == key {
			return tabkeyval[1]
		}
	}
	return ""
}

func Replaceval(msg string, key string, new_val string) string {
	if len(msg) < 2 {
		stderr.Print(Format_w("message.go", "Replaceval", "message trop court : "+msg))
		return ""
	}
	sep := msg[0:1]
	tab_allkeyvals := strings.Split(msg[1:], sep)
	for i, keyval := range tab_allkeyvals {
		if len(keyval) < 2 {
			continue
		}
		equ := keyval[0:1]
		tabKeyVal := strings.SplitN(keyval[1:], equ, 2)
		if len(tabKeyVal) != 2 {
			continue
		}
		if tabKeyVal[0] == key {
			// Replace value
			tab_allkeyvals[i] = equ + key + equ + new_val
			break
		}
	}
	// Reconstruct the message
	return sep + strings.Join(tab_allkeyvals, sep)

}

func Msg_send(msg string, p_nom string) {
	if strings.Contains(msg, "pear_discovery") == false {
		// Only print to stderr msg that are not related to pear_discovery
		// (overwhelming)
		stderr.Printf(Format_w(p_nom, "Msg_send()", "émission de "+msg))
	}
	fmt.Print(msg + "\n")
}

var fieldsep = "/"
var keyvalsep = "="

func Msg_format(key string, val string) string {
	return fieldsep + keyvalsep + key + keyvalsep + val
}


// Build_msg_args(key1, value1, key2, value2, ...) aims to create a string structure as :
//  map[string]string{
//	    "user": "alice",
//	    "id":   "1234",
//	    "role": "admin",
// }
// by calling Build_msg_args("user", "alice", "id", "1234",...)
// This can then be passed to Msg_format_multi
func Build_msg_args(args ...string) map[string]string {
	data := make(map[string]string)
	for i := 0; i < len(args); i += 2 {
		key := args[i]
		val := args[i+1]
		data[key] = val
	}

	// Check mandatory keys: at least sender_name and clk need
	// to be present in the message = they have to be
	// given as input to Build_msg_args
	var mandatory_keys = []string{"sender_name", "clk", "destination", "id"}
	for _, key := range mandatory_keys {
		if _, ok := data[key]; !ok {
			log.Fatal(Format_e(
				"message.go",
				"Build_msg_args",
				"missing_mandatory_key: " + key))
		}
	}
	return data
}

// Msg_format_multi formats a message using multiple keys and multiple 
// values (1 key - 1 value).
// Usage :
// 	data:= format.Build_msg_args("sender_name", *sensor_name, "clk2", strconv.Itoa(clk))
// 	formatted := Msg_format_multi(data)
func Msg_format_multi(kvPairs map[string]string) string {
	result := ""
	for key, val := range kvPairs {
		result += fieldsep + keyvalsep + key + keyvalsep + val
	}
	return result
}
