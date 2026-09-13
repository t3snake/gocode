package core

import "errors"

var CancelSignalError = errors.New("Communication to the LLM interrupted")
