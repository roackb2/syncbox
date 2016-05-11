package syncbox

import "errors"

// custom errors for this application
var (
	ErrorEmptyContent       = errors.New("empty content")
	ErrorUnknownRequestType = errors.New("unknown request type")
	ErrorPeerSocketClosed   = errors.New("peer socket closed")
)
