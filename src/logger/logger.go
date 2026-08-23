package logger

import (
	"log"
	"os"
	"sync"
)

var logger *log.Logger = nil
var once sync.Once

func Init(file *os.File) {
	once.Do(func() {
		logger = log.New(file, "LOG", log.Ldate|log.Ltime|log.Lshortfile)
	})
}

func Info(info string) {
	logger.Printf("[INFO] %s\n", info)
}

func Error(err string) {
	logger.Printf("[ERROR] %s\n", err)
}

func Warning(warn string) {
	logger.Printf("[WARN] %s\n", warn)
}
