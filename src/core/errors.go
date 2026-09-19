package core

import "errors"

var CancelSignalError = errors.New("communication to the LLM interrupted")
