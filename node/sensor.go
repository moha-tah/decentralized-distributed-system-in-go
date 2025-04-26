package node 

import (
	"fmt"
	"time"
	"strconv"
	"distributed_system/format"
)

func Sensor(sensor_name *string) {
	var clk int = 0 // clock
	for {
		clk  = clk + 1

		data := format.Build_msg_args("sender_name", *sensor_name, "clk", strconv.Itoa(clk))
		

		formatted := format.Msg_format_multi(data)
		fmt.Println(formatted)

	      
		time.Sleep(time.Duration(2) * time.Second)
	}
}
