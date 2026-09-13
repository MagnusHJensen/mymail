package smtp

import "fmt"

type ReplyError struct {
	code Code
	msg  string
	error
}

func NewReplyError(code Code, msg string) *ReplyError {
	return &ReplyError{
		code: code,
		msg:  msg,
	}
}

func (e *ReplyError) Error() string {
	return fmt.Sprintf("%d %s", e.code, e.msg)
}

func (e *ReplyError) GetCode() Code {
	return e.code
}

func (e *ReplyError) GetMsg() string {
	return e.msg
}
