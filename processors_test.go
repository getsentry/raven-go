package raven

import "testing"

func TestScrubTags(t *testing.T) {
	packet := &Packet{Tags: []Tag{
		Tag{"safe", "word"},
		Tag{"pass", "word"},
		Tag{"password", "nope"},
		Tag{"passwd", "nope"},
		Tag{"secret", "word"},
		Tag{"hallpass", "bathroom"},
		Tag{"cc", "4242 4242 4242 4242"},
		Tag{"cc", "4012-8888-8888-1881"},
		Tag{"cc", "371449635398431"},
	}}

	cleanPacket := scrubTags(packet)

	if len(packet.Tags) != 9 {
		t.Fatal("not all the original tags made it")
	}
	if packet.Tags[0].Value != "word" {
		t.Error("original safe tags were modified")
	}
	if packet.Tags[1].Value != "word" {
		t.Error("original unsafe tags were modified")
	}
	if packet.Tags[6].Value != "4242 4242 4242 4242" {
		t.Error("original unsafe credit card was modified")
	}
	if len(cleanPacket.Tags) != 9 {
		t.Fatalf("not all the cleaned tags made it: only %d", len(cleanPacket.Tags))
	}
	if cleanPacket.Tags[0].Value != "word" {
		t.Error("cleaned safe tag was modified")
	}
	if cleanPacket.Tags[1].Value != "********" {
		t.Errorf("unsafe tag %s wasn't scrubbed", cleanPacket.Tags[1].Key)
	}
	if cleanPacket.Tags[2].Value != "********" {
		t.Errorf("unsafe tag %s wasn't scrubbed", cleanPacket.Tags[2].Key)
	}
	if cleanPacket.Tags[3].Value != "********" {
		t.Errorf("unsafe tag %s wasn't scrubbed", cleanPacket.Tags[3].Key)
	}
	if cleanPacket.Tags[4].Value != "********" {
		t.Errorf("unsafe tag %s wasn't scrubbed", cleanPacket.Tags[4].Key)
	}
	if cleanPacket.Tags[5].Value != "bathroom" {
		t.Error("cleaned safe tag was modified")
	}
	if cleanPacket.Tags[6].Value != "****************" {
		t.Error("cleaned unsafe credit card wasn't scrubbed")
	}
	if cleanPacket.Tags[7].Value != "****************" {
		t.Error("cleaned unsafe credit card wasn't scrubbed")
	}
	if cleanPacket.Tags[8].Value != "****************" {
		t.Error("cleaned unsafe credit card wasn't scrubbed")
	}
}

func TestScrubExtra(t *testing.T) {
	packet := &Packet{Extra: map[string]interface{}{
		"safe":     "word",
		"pass":     "word",
		"password": "nope",
		"passwd":   "nope",
		"secret":   "word",
		"hallpass": "bathroom",
		"cc1":      "4242 4242 4242 4242",
		"cc2":      "4012-8888-8888-1881",
		"cc3":      "371449635398431",
	}}

	cleanPacket := scrubExtra(packet)

	if len(packet.Extra) != 9 {
		t.Fatal("not all the original extra info made it")
	}
	if packet.Extra["safe"] != "word" {
		t.Error("original safe key was modified")
	}
	if packet.Extra["pass"] != "word" {
		t.Error("original unsafe key was modified")
	}
	if packet.Extra["cc2"] != "4012-8888-8888-1881" {
		t.Error("original unsafe key was modified")
	}
	if len(cleanPacket.Extra) != 9 {
		t.Fatalf("not all the cleaned tags made it: only %d", len(cleanPacket.Extra))
	}
	if cleanPacket.Extra["safe"] != "word" {
		t.Error("cleaned safe tag was modified")
	}
	if cleanPacket.Extra["pass"] != "********" {
		t.Error("unsafe extra \"pass\" wasn't scrubbed")
	}
	if cleanPacket.Extra["password"] != "********" {
		t.Error("unsafe extra \"password\" wasn't scrubbed")
	}
	if cleanPacket.Extra["passwd"] != "********" {
		t.Error("unsafe extra \"passwd\" wasn't scrubbed")
	}
	if cleanPacket.Extra["secret"] != "********" {
		t.Error("unsafe extra \"secret\" wasn't scrubbed")
	}
	if cleanPacket.Extra["hallpass"] != "bathroom" {
		t.Error("cleaned safe extra \"hallpass\" was modified")
	}
	if cleanPacket.Extra["cc1"] != "****************" {
		t.Error("unsafe extra \"cc1\" wasn't scrubbed")
	}
	if cleanPacket.Extra["cc2"] != "****************" {
		t.Error("unsafe extra \"cc2\" wasn't scrubbed")
	}
	if cleanPacket.Extra["cc3"] != "****************" {
		t.Error("unsafe extra \"cc3\" wasn't scrubbed")
	}
}
