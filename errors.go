package syncbox

import "errors"

// custom errors for this application
var (
	ErrorEmptyContent       = errors.New("empty content")
	ErrorUnknownRequestType = errors.New("unknown request type")
	ErrorPeerSocketClosed   = errors.New("peer socket closed")
	ErrorExceedsAddrLength  = errors.New("exceeds address length")
	ErrorNoFileRecords      = errors.New("no file records found")
	ErrorTimeout            = errors.New("operation timeout")
	ErrorRequestNotFound    = errors.New("request not found")
)
