package utils

import (
	"fmt"
	"log"
)

// Logger represents a logging utility with unique channels for different log types
// and an identifier for the logger instance.
type Logger struct {
	Stdout chan string       // Channel for standard log messages.
	Stderr chan string       // Channel for error log messages.
	File   chan FileWithInfo // Channel for file log messages.
	Cmd    chan string       // Channel for command log messages.
	Id     string            // Identifier for the logger instance.
}

// FileWithInfo represents a file log message with its file name and content.
type FileWithInfo struct {
	FileName string // Name of the file being logged.
	Content  string // Content of the file being logged.
}

// NewLogger initializes a new Logger instance with unique channels and an identifier.
//
// Parameters:
//   - id: A string representing the identifier for the logger instance.
//
// Returns:
//   - A pointer to the newly created Logger instance.
func NewLogger(id string) *Logger {
	return &Logger{
		Stdout: make(chan string, 100),
		Stderr: make(chan string, 100),
		File:   make(chan FileWithInfo, 100),
		Cmd:    make(chan string, 100),
		Id:     id,
	}
}

// Log formats a log message and sends it to the Stdout channel.
//
// Parameters:
//   - format: A string containing the format of the log message (similar to fmt.Sprintf).
//   - args: A variadic list of arguments to be formatted into the log message.
func (l *Logger) Log(format string, args ...interface{}) {
	l.Stdout <- fmt.Sprintf(format, args...)
}

// LogErr formats an error log message and sends it to the Stderr channel.
//
// Parameters:
//   - format: A string containing the format of the error log message (similar to fmt.Sprintf).
//   - args: A variadic list of arguments to be formatted into the error log message.
func (l *Logger) LogErr(format string, args ...interface{}) {
	l.Stderr <- fmt.Sprintf(format, args...)
}

// LogFile formats a log message and sends it to the File channel.
//
// Parameters:
//   - filePath: A string representing the path of the file being logged.
//   - content: A string containing the content of the file being logged.
func (l *Logger) LogFile(filePath, content string) {
	l.File <- FileWithInfo{FileName: filePath, Content: content}
}

// LogCmd formats a command log message and sends it to the Cmd channel.
//
// Parameters:
//   - format: A string containing the format of the command log message (similar to fmt.Sprintf).
//   - args: A variadic list of arguments to be formatted into the command log message.
func (l *Logger) LogCmd(format string, args ...interface{}) {
	l.Cmd <- fmt.Sprintf(format, args...)
}

// LogWorker continuously processes log messages from the Stdout channel
// and writes them to the standard logger.
//
// This function should be run as a goroutine to handle log messages asynchronously.
func (l *Logger) LogWorker() {
	for logMessage := range l.Stdout {
		log.Println(fmt.Sprintf("[INFO] %s", logMessage))
	}
}

// LogWorkerErr continuously processes error log messages from the Stderr channel
// and writes them to the standard logger with an error prefix.
//
// This function should be run as a goroutine to handle error log messages asynchronously.
func (l *Logger) LogWorkerErr() {
	for logMessage := range l.Stderr {
		log.Println(fmt.Sprintf("[ERROR] %s", logMessage))
	}
}

// LogWorkerFile continuously processes log messages from the File channel
// and writes them to the standard logger with a file prefix.
//
// This function should be run as a goroutine to handle file log messages asynchronously.
func (l *Logger) LogWorkerFile() {
	delimiter := "----------------------------------------"
	for logMessage := range l.File {
		strings := []string{delimiter, logMessage.FileName, delimiter, logMessage.Content, delimiter, logMessage.FileName, delimiter}
		log.Println("[FILE]")
		for _, s := range strings {
			log.Println(s)
		}
	}
}

// LogWorkerCmd continuously processes command log messages from the Cmd channel
// and writes them to the standard logger with a command prefix.
//
// This function should be run as a goroutine to handle command log messages asynchronously.
func (l *Logger) LogWorkerCmd() {
	for logMessage := range l.Cmd {
		log.Println(fmt.Sprintf("[CMD] %s", logMessage))
	}
}
