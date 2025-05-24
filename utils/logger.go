package utils

import (
	"fmt"
	"log"
)

type Logger struct {
	Stdout chan string
	Stderr chan string
	File   chan FileWithInfo
	Cmd    chan string
	Id     string
}

type FileWithInfo struct {
	FileName string
	Content  string
}

func NewLogger(id string) *Logger {
	return &Logger{
		Stdout: make(chan string, 100),
		Stderr: make(chan string, 100),
		File:   make(chan FileWithInfo, 100),
		Cmd:    make(chan string, 100),
		Id:     id,
	}
}

func (l *Logger) Log(format string, args ...interface{}) {
	l.Stdout <- fmt.Sprintf(format, args...)
}

func (l *Logger) LogErr(format string, args ...interface{}) {
	l.Stderr <- fmt.Sprintf(format, args...)
}
func (l *Logger) LogFile(filePath, content string) {
	l.File <- FileWithInfo{FileName: filePath, Content: content}
}
func (l *Logger) LogCmd(format string, args ...interface{}) {
	l.Cmd <- fmt.Sprintf(format, args...)
}
func (l *Logger) LogWorker() {
	for logMessage := range l.Stdout {
		log.Println(fmt.Sprintf("[stdout] %s", logMessage))
	}
}
func (l *Logger) LogWorkerErr() {
	for logMessage := range l.Stderr {
		log.Println(fmt.Sprintf("[stderr] %s", logMessage))
	}
}
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
func (l *Logger) LogWorkerCmd() {
	for logMessage := range l.Cmd {
		log.Println(fmt.Sprintf("[CMD] %s", logMessage))
	}
}
