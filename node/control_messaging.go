package node

import (
	"distributed_system/format"
	"distributed_system/utils"
	"strconv"
	"strings"
	"time"
)

// Propagate a message to all other nodes
func (c *ControlLayer) propagateMessage(msg string) {
	var propagate_msg string = format.Replaceval(msg, "sender_name", c.GetName())
	propagate_msg = format.Replaceval(propagate_msg, "sender_type", "control")
	c.SendMsg(propagate_msg)
}

// AddNewMessageId adds an entry in the seenIDs to remember
// that the control layer saw this message.
func (c *ControlLayer) AddNewMessageId(sender_name string, MID_str string) {
	msg_NbMessageSent, err := format.MIDFromString(MID_str)
	if err != nil {
		format.Display(format.Format_e("AddNewMessageID()", c.GetName(), "Error in message id: "+err.Error()))
	}

	c.mu.Lock()
	c.IDWatcher.AddMIDToNode(sender_name, msg_NbMessageSent)
	c.mu.Unlock()
}

// SendMsgFromApplication is the portal between control layer and application
// layer: the app layer asks the control layer to send a message to the network.
// It is supposed that the application won't send two times the same message,
// so no check if already got the message (=already seen the ID).
// => Is it a receiving action followed by a call to sending action
func (c *ControlLayer) SendApplicationMsg(msg string) error {
	c.mu.Lock()
	// app necessarily has a vector clock if it has started
	recVC := format.RetrieveVectorClock(msg, len(c.vectorClock))
	c.vectorClock = utils.SynchroniseVectorClock(c.vectorClock, recVC, c.nodeIndex)
	c.mu.Unlock()

	c.SendMsg(msg)

	return nil
}

// Send a Message (from this control layer). If is it a propagation, then
// set `fixed_clock` to the same clock as received message to propagate.
func (c *ControlLayer) SendControlMsg(msg_content string, msg_content_type string,
	msg_type string, destination string, fixed_id string,
	sender_name_source string) error {
	var msg_id string
	if fixed_id == "" {
		msg_id = c.GenerateUniqueMessageID()
	} else {
		msg_id = fixed_id
	}

	msg := format.Msg_format_multi(
		format.Build_msg_args(
			"id", msg_id,
			"type", msg_type,
			"sender_name", c.GetName(),
			"sender_name_source", sender_name_source,
			"sender_type", "control",
			"destination", destination,
			"clk", "", // Will be replaced in c.SendMsg
			"vector_clock", "", // Will be replaced in c.SendMsg
			"content_type", msg_content_type,
			"content_value", msg_content))

	c.SendMsg(msg)

	return nil
}

// SendMsg sends a message to the network.
// Param through_channel is used to indicate if the message
// should be sent through the channel to the application layer ONLY (ie.
// not to the network).
func (c *ControlLayer) SendMsg(msg string, through_channelArgs ...bool) {
	through_channel := false
	if len(through_channelArgs) > 0 {
		through_channel = through_channelArgs[0]
	}

	// As this node sends the message, it doens't want to receive
	// a duplicate => add the message ID to ID watcher
	var msg_id_str string = format.Findval(msg, "id")
	var msg_splits []string = strings.Split(msg_id_str, "_")
	var msg_NbMessageSent_str string = msg_splits[len(msg_splits)-1]
	// The sender can also be the app layer, so check for that:
	var msg_sender string = format.Findval(msg, "sender_name_source")
	c.AddNewMessageId(msg_sender, msg_NbMessageSent_str)

	c.mu.Lock()
	if c.vectorClockReady {
		c.vectorClock[c.nodeIndex] += 1 // Incrément de l'horloge vectorielle locale
		msg = format.Replaceval(msg, "vector_clock", utils.SerializeVectorClock(c.vectorClock))
	}
	c.clk += 1 // Incrément de l'horloge locale
	msg = format.Replaceval(msg, "clk", strconv.Itoa(c.clk))

	if !strings.Contains(msg, "snapshot_id") {
		msg = format.AddFieldToMessage(msg, "snapshot_id", strconv.Itoa(c.nodeMarker))
	}
	c.mu.Unlock()
	if through_channel {
		select {
		case c.channel_to_application <- msg:
			// Message sent successfully
		case <-time.After(100 * time.Millisecond):
			format.Display(format.Format_w(c.GetName(), "SendMsg()", "Channel appears to be blocked"))
		}
	} else {
		// format.Msg_send(msg, c.GetName())
		// c.networkLayer.SendMessage(msg, -1)
		msg = format.AddFieldToMessage(msg, "sender_id", c.id)
		c.SendMsgToNetwork(msg)
	}

	c.mu.Lock()
	c.nbMsgSent = c.nbMsgSent + 1
	c.mu.Unlock()

}

func (c *ControlLayer) SendMsgToNetwork(msg string) {
	c.mu.Lock()
	networkLayer := c.networkLayer

	msg = format.AddOrReplaceFieldToMessage(msg, "id", c.GenerateUniqueMessageID())
	c.mu.Unlock()

	networkLayer.MessageFromControlLayer(msg)

	c.mu.Lock()
	c.nbMsgSent = c.nbMsgSent + 1
	c.mu.Unlock()

}

// Return true if the message's ID is contained with
// one of the controler's ID pairs. If it is not contains,
// it adds it to the interval (see utils.message_watch)
// and return false.
func (c *ControlLayer) SawThatMessageBefore(msg string) bool {
	var msg_id_str string = format.Findval(msg, "id")

	var msg_splits []string = strings.Split(msg_id_str, "_")
	var msg_NbMessageSent_str string = msg_splits[len(msg_splits)-1]

	msg_NbMessageSent, err := format.MIDFromString(msg_NbMessageSent_str)
	if err != nil {
		format.Display(format.Format_e("SawThatMessageBefore()", c.GetName(), "Error in message id: "+err.Error()))
	}

	var sender_name string = format.Findval(msg, "sender_name_source")

	// Never saw that message before
	c.mu.Lock()
	if !c.IDWatcher.ContainsMID(sender_name, msg_NbMessageSent) {
		c.IDWatcher.AddMIDToNode(sender_name, msg_NbMessageSent)
		c.mu.Unlock()
		return false
	}
	c.mu.Unlock()
	// Saw that message before as it is contained in intervals:
	return true
}
