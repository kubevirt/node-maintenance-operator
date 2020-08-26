package e2e

import (
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
)

const (
	INFO = "INFO"
	WARN = "WARN"
)

func logInfoln(arg string) {
	log(INFO, fmt.Sprintf("%s\n", arg))
}

func logInfof(format string, args ...interface{}) {
	log(INFO, fmt.Sprintf(format, args...))
}

func logWarnln(arg string) {
	log(WARN, fmt.Sprintf("%s\n", arg))
}

func logWarnf(format string, args ...interface{}) {
	log(WARN, fmt.Sprintf(format, args...))
}

func log(prefix, arg string) {
	time := time.Now().Format(time.RFC3339)
	msg := fmt.Sprintf("%s: [%s] %s", time, prefix, arg)
	_, _ = ginkgo.GinkgoWriter.Write([]byte(msg))
}
