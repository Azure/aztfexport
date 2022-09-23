package internal

import "fmt"

// Abstract the Messager struct in the github.com/magodo/spinner
type Messager interface {
	SetStatus(msg string)
	SetDetail(msg string)
}

type StdoutMessager struct{}

func (p *StdoutMessager) SetStatus(msg string) {
	fmt.Println(msg)
}

func (p *StdoutMessager) SetDetail(msg string) {
	fmt.Println(msg)
}
