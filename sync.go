package syncbox

// Constants for the action to sync
const (
	ActionAdd    = "add"
	ActionDelete = "delete"
	ActionRename = "rename"
	ActionUpdate = "update"
	ActionGet    = "get"
)

// FileManipulator function type that do CRUD on files
type FileManipulator func(path string, file *File, peer *Peer) error

// Syncer is the interface to send file CRUD requests
type Syncer interface {
	AddFile(path string, file *File, peer *Peer) error
	DeleteFile(path string, file *File, peer *Peer) error
	GetFile(path string, file *File, peer *Peer) error
	AddDir(path string, dir *Dir, peer *Peer) error
	DeleteDir(path string, dir *Dir, peer *Peer) error
}

// FileManipRequest represents operations to do on files
type FileManipRequest struct {
	Action string
	File   *File
}
