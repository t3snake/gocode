package logger

import (
	"fmt"
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

func Infof(info_format string, v ...any) {
	Info(fmt.Sprintf(info_format, v...))
}

func Error(err string) {
	logger.Printf("[ERROR] %s\n", err)
}

func Errorf(error_format string, v ...any) {
	Error(fmt.Sprintf(error_format, v...))
}

func Warning(warn string) {
	logger.Printf("[WARN] %s\n", warn)
}

func Warningf(warn_format string, v ...any) {
	Warning(fmt.Sprintf(warn_format, v...))
}
