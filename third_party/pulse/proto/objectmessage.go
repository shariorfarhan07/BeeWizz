package proto

// Local addition (not in upstream jfreymuth/pulse v0.1.3): the
// PA_COMMAND_SEND_OBJECT_MESSAGE request added in PulseAudio 15
// (protocol v34). It is what `pactl send-message` uses, e.g. to list
// and switch Bluetooth codecs via /card/<name>/bluez.

const OpSendObjectMessage = 104

// SendObjectMessage sends Message to the handler registered at
// ObjectPath. Parameters is a JSON document; an empty string is sent
// as NULL (no parameters).
type SendObjectMessage struct {
	ObjectPath string
	Message    string
	Parameters string
}

// SendObjectMessageReply carries the handler's JSON response.
type SendObjectMessageReply struct {
	Response string
}

func (*SendObjectMessage) command() uint32        { return OpSendObjectMessage }
func (*SendObjectMessageReply) IsReplyTo() uint32 { return OpSendObjectMessage }
