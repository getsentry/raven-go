package raven

import "regexp"

var CreditCardRegex = regexp.MustCompile(`^(?:\d[ -]*?){13,16}$`)

type Processor func(*Packet) *Packet

func scrubTags(packet *Packet) *Packet {
	scrubbedPacket := *packet

	scrubbedPacket.Tags = make([]Tag, 0, len(packet.Tags))

	for _, tag := range packet.Tags {
		newTag := tag

		switch newTag.Key {
		case "password", "passwd", "pass", "secret":
			newTag.Value = "********"
		}

		if CreditCardRegex.MatchString(newTag.Value) {
			newTag.Value = "****************"
		}

		scrubbedPacket.Tags = append(scrubbedPacket.Tags, newTag)
	}

	return &scrubbedPacket
}

func scrubExtra(packet *Packet) *Packet {
	scrubbedPacket := *packet
	scrubbedPacket.Extra = make(map[string]interface{}, len(packet.Extra))

	for key, value := range packet.Extra {
		switch key {
		case "password", "passwd", "pass", "secret":
			scrubbedPacket.Extra[key] = "********"
		default:
			scrubbedPacket.Extra[key] = value
		}

		if strVal, ok := value.(string); ok {
			if CreditCardRegex.MatchString(strVal) {
				scrubbedPacket.Extra[key] = "****************"
			}
		}
	}

	return &scrubbedPacket
}
