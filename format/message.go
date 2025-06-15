package format

import (
	"distributed_system/consts"
	"distributed_system/utils"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
)

func Findval(msg string, key string) string {
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

func AddFieldToMessage(msg string, key string, val string) string {
	return msg + fieldsep + keyvalsep + key + keyvalsep + val
	// sep := msg[0:1]
	// tab_allkeyvals := strings.Split(msg[1:], sep)
	// tab_allkeyvals = append(tab_allkeyvals, sep+keyvalsep+key+keyvalsep+val)
	// return sep + strings.Join(tab_allkeyvals, sep)
}

func AddOrReplaceFieldToMessage(msg string, key string, val string) string {
	if strings.Contains(msg, keyvalsep+key+keyvalsep) {
		// Key already exists, replace its value
		return Replaceval(msg, key, val)
	} else {
		// Key does not exist, add it
		return AddFieldToMessage(msg, key, val)
	}
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
		// stderr.Printf(Format_w(p_nom, "Msg_send()", " => "+msg))
	}
	fmt.Print(msg + "\n")
}

var fieldsep = consts.Fieldsep
var keyvalsep = consts.Keyvalsep

func Msg_format(key string, val string) string {
	return fieldsep + keyvalsep + key + keyvalsep + val
}

// Build_msg_args(key1, value1, key2, value2, ...) aims to create a string structure as :
//
//	 map[string]string{
//		    "user": "alice",
//		    "id":   "1234",
//		    "role": "admin",
//	}
//
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
	// var mandatory_keys = []string{"sender_name", "destination", "id"}
	// for _, key := range mandatory_keys {
	// 	if _, ok := data[key]; !ok {
	// 		log.Fatal(Format_e(
	// 			"message.go",
	// 			"Build_msg_args",
	// 			"missing_mandatory_key: " + key))
	// 	}
	// }
	return data
}

// Msg_format_multi formats a message using multiple keys and multiple
// values (1 key - 1 value). Message is sorted alphabetically by keys
// Usage :
//
//	data:= format.Build_msg_args("sender_name", *sensor_name, "clk2", strconv.Itoa(clk))
//	formatted := Msg_format_multi(data)
func Msg_format_multi(kvPairs map[string]string) string {
	result := ""
	keys := make([]string, 0, len(kvPairs))

	// Collect and sort the keys
	for key := range kvPairs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Iterate over sorted keys and build the result string
	for _, key := range keys {
		result += fieldsep + keyvalsep + key + keyvalsep + kvPairs[key]
	}
	return result
}

func RetrieveVectorClock(msg string, length_to_compare int) []int {
	recVC_str := Findval(msg, "vector_clock")
	recVC, err := utils.DeserializeVectorClock(recVC_str)
	if err != nil {
		stderr.Print(Format_e("message.go", "retrieveVectorClock", "Error of vector_clock deserialization: "+err.Error()))
		return nil
	}
	if len(recVC) != length_to_compare {
		stderr.Print(Format_e("message.go", "retrieveVectorClock", "Error of vector_clock length: "+strconv.Itoa(len(recVC_str))+"vs "+strconv.Itoa(length_to_compare)))
		return nil
	}
	return recVC
}

func Build_msg(args ...string) string {
	if len(args) < 2 || len(args)%2 != 0 {
		log.Fatal(Format_e("message.go", "Build_msg", "args must be even and at least 2"))
	}
	data := Build_msg_args(args...)
	return Msg_format_multi(data)
}
