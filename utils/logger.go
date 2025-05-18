package utils

import (
	"fmt"
	"log"
)

// LogChannel is a buffered channel used to queue log messages for processing.
var LogChannel = make(chan string, 100)

// Log formats a log message and sends it to the LogChannel for processing.
//
// Parameters:
//   - format: A string containing the format of the log message (similar to fmt.Sprintf).
//   - args: A variadic list of arguments to be formatted into the log message.
func Log(format string, args ...interface{}) {
	LogChannel <- fmt.Sprintf(format, args...)
}

// LogWorker continuously processes log messages from the LogChannel and writes them to the standard logger.
//
// This function should be run as a goroutine to handle log messages asynchronously.
func LogWorker() {
	for logMessage := range LogChannel {
		log.Println(logMessage)
	}
}
