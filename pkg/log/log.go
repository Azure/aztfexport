package log

type Logger interface {
	Printf(format string, v ...any)
}

var log Logger = NullLogger{}

type NullLogger struct{}

func (NullLogger) Printf(format string, v ...any) {}

func SetLogger(l Logger) {
	log = l
}

func Printf(format string, v ...any) {
	log.Printf(format, v...)
}
