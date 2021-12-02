package lib

import "log"

// TODO: Expose a logger interface that can be implemented by the user
// A logger interface lets user implement how and where library logs are processed

// Debug enables verbose logging in the library
var Debug = false

// debugLog logs a message to stdout if Debug is set to true
func debugLog(format string, v ...interface{}) {
	if Debug {
		log.Printf(format, v...)
	}
}
