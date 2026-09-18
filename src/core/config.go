package core

import "time"

// currently contains all hardcoded constants

const Version = "0.0.1"

const StreamTimeout = 5 * time.Minute
const ToolExecutionTimeout = 5 * time.Minute

const MessageSizeLimit = 255

const ToolResultLogTruncLimit = 300 // The number of characters after which the log truncates
