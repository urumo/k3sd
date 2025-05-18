package utils

import (
	"fmt"
	"log"
)

var LogChannel chan string = make(chan string, 100)

func Log(format string, args ...interface{}) {
	LogChannel <- fmt.Sprintf(format, args...)
}

func LogWorker() {
	for {
		select {
		case logMessage := <-LogChannel:
			log.Println(logMessage)
		}
	}
}
