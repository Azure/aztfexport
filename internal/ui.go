package internal

import (
	"log"
	"os"
)

// Abstract the Messager struct in the github.com/magodo/spinner
type Messager interface {
	SetStatus(msg string)
	SetDetail(msg string)
}

type stdoutMessager struct {
	*log.Logger
}

func NewStdoutMessager() Messager {
	return &stdoutMessager{
		Logger: log.New(os.Stdout, "[aztfexport] ", log.LstdFlags),
	}
}

func (p *stdoutMessager) SetStatus(msg string) {
	p.Println(msg)
}

func (p *stdoutMessager) SetDetail(msg string) {
	p.Println(msg)
}
