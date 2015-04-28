package raven

import (
	"net/url"
	"regexp"
	"strings"
)

const Mask = "********"

var querySecretKeys = []string{"api_key", "apikey", "authorization", "passwd", "password", "secret"}
var querySecretValues = []string{`/^(?:\d[ -]*?){13,16}$/`}

// Scrub all data for a packet
func (client *Client) Scrub(packet *Packet) *Packet {

	packet = defaultProcessor(packet)
	for _, processor := range *client.Config.Processors {
		packet = processor(packet)
	}
	return packet
}

// Default processor for a packet
func defaultProcessor(packet *Packet) *Packet {
	for _, packetInterface := range packet.Interfaces {
		switch typedInterface := packetInterface.(type) {
		case *Http:
			scrubStringMap(typedInterface.Headers)
		default:
			continue
		}
	}
	return packet
}

// Scrubs map of string -> string
func scrubStringMap(stringMap map[string]string) map[string]string {
	// Loops through the map and scrubs and sensitive data
	for key, val := range stringMap {
		stringMap[key] = scrubKeyValuePair(key, val)
	}
	return stringMap
}

// Check key/value pair for sensitive data
func scrubKeyValuePair(key, val string) string {

	if keyIsSensitive(key) {
		return Mask
	}

	if valIsSensitive(val) {
		return Mask
	}

	return val
}

// Check keys for sensitive data, matches list of substrings
func keyIsSensitive(key string) (sensitive bool) {
	for _, secretKey := range querySecretKeys {
		// Make lower for case insensitive compare
		key = strings.ToLower(key)
		if strings.Contains(key, secretKey) {
			return true
		}
	}
	return false
}

// Check values for sensitive data, matches regex list
func valIsSensitive(val string) (sensitive bool) {
	for _, regex := range querySecretValues {
		// Note: This will panic if querySecretValues has a bad regex
		regexMatcher := regexp.MustCompile(regex)
		if regexMatcher.MatchString(val) {
			return true
		}
	}
	return false
}

// Sanitize the query before sending it
func scrubQuery(query url.Values) url.Values {

	for key, values := range query {
		for index, val := range values {
			// Check key
			if keyIsSensitive(key) {
				query[key] = []string{Mask}
			}

			// Check value
			if valIsSensitive(val) {
				query[key][index] = Mask
			}
		}
	}

	return query
}
